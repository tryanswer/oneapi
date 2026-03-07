package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/common/logger"
	"github.com/songquanpeng/one-api/model"
	billingratio "github.com/songquanpeng/one-api/relay/billing/ratio"
	"github.com/songquanpeng/one-api/relay/channeltype"
)

const (
	videoProviderAli          = "ali"
	videoStatusQueued         = "queued"
	videoStatusRunning        = "running"
	videoStatusSucceeded      = "succeeded"
	videoStatusFailed         = "failed"
	videoStatusCancelled      = "cancelled"
	videoStatusUnknown        = "unknown"
	videoEndpointTextToVideo  = "/api/v1/services/aigc/video-generation/video-synthesis"
	videoEndpointImageToVideo = "/api/v1/services/aigc/image2video/video-synthesis"
)

type VideoGenerationRequest struct {
	Model           string  `json:"model"`
	Prompt          string  `json:"prompt"`
	Image           *string `json:"image,omitempty"`
	FirstFrameImage *string `json:"first_frame_image,omitempty"`
	LastFrameImage  *string `json:"last_frame_image,omitempty"`
	Size            string  `json:"size,omitempty"`
	Duration        int     `json:"duration,omitempty"`
	ResponseFormat  string  `json:"response_format,omitempty"`
	NegativePrompt  string  `json:"negative_prompt,omitempty"`
}

type aliVideoGenerationRequest struct {
	Model string `json:"model"`
	Input struct {
		Prompt        string `json:"prompt,omitempty"`
		ImgURL        string `json:"img_url,omitempty"`
		FirstFrameURL string `json:"first_frame_url,omitempty"`
		LastFrameURL  string `json:"last_frame_url,omitempty"`
	} `json:"input"`
	Parameters struct {
		Size         string `json:"size,omitempty"`
		Resolution   string `json:"resolution,omitempty"`
		Duration     int    `json:"duration,omitempty"`
		PromptExtend bool   `json:"prompt_extend,omitempty"`
		Watermark    bool   `json:"watermark,omitempty"`
	} `json:"parameters,omitempty"`
}

type aliVideoTaskResponse struct {
	StatusCode int    `json:"status_code,omitempty"`
	RequestID  string `json:"request_id,omitempty"`
	Code       string `json:"code,omitempty"`
	Message    string `json:"message,omitempty"`
	Output     struct {
		TaskID       string `json:"task_id,omitempty"`
		TaskStatus   string `json:"task_status,omitempty"`
		VideoURL     string `json:"video_url,omitempty"`
		Code         string `json:"code,omitempty"`
		Message      string `json:"message,omitempty"`
		SubmitTime   string `json:"submit_time,omitempty"`
		ScheduleTime string `json:"scheduled_time,omitempty"`
		EndTime      string `json:"end_time,omitempty"`
	} `json:"output,omitempty"`
	Usage any `json:"usage,omitempty"`
}

type videoTaskResponse struct {
	ID        string          `json:"id"`
	Object    string          `json:"object"`
	Status    string          `json:"status"`
	Model     string          `json:"model"`
	Provider  string          `json:"provider"`
	ChannelID int             `json:"channel_id,omitempty"`
	Data      []videoTaskURL  `json:"data,omitempty"`
	Error     *videoTaskError `json:"error,omitempty"`
}

type videoTaskURL struct {
	URL string `json:"url"`
}

type videoTaskError struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

func CreateVideoGenerationTask(c *gin.Context) {
	ctx := c.Request.Context()
	var req VideoGenerationRequest
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		writeVideoError(c, http.StatusBadRequest, "invalid_request_error", "invalid_video_generation_request", "Invalid video generation request body")
		return
	}
	if req.Model == "" {
		writeVideoError(c, http.StatusBadRequest, "invalid_request_error", "model_missing", "model is required")
		return
	}
	if req.Prompt == "" {
		writeVideoError(c, http.StatusBadRequest, "invalid_request_error", "prompt_missing", "prompt is required")
		return
	}

	channelID := c.GetInt(ctxkey.ChannelId)
	channel, err := model.GetChannelById(channelID, true)
	if err != nil {
		writeVideoError(c, http.StatusBadRequest, "one_api_error", "channel_not_found", "Selected channel not found")
		return
	}
	if !isAliVideoChannel(channel.Type) {
		writeVideoError(c, http.StatusBadRequest, "invalid_request_error", "provider_not_supported_for_video_generation", "Current channel does not support video generation yet")
		return
	}
	quota, quotaErr := estimateVideoTaskQuota(c, channel, req.Model)
	if quotaErr != nil {
		writeVideoError(c, quotaErr.status, quotaErr.errType, quotaErr.code, quotaErr.message)
		return
	}

	aliReq, endpoint, err := buildAliVideoGenerationRequest(req)
	if err != nil {
		writeVideoError(c, http.StatusBadRequest, "invalid_request_error", "invalid_video_generation_request", err.Error())
		return
	}
	payload, err := json.Marshal(aliReq)
	if err != nil {
		writeVideoError(c, http.StatusInternalServerError, "one_api_error", "marshal_video_generation_request_failed", "Failed to encode video generation request")
		return
	}

	baseURL := normalizeBaseURL(channel.GetBaseURL())
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+endpoint, bytes.NewReader(payload))
	if err != nil {
		writeVideoError(c, http.StatusInternalServerError, "one_api_error", "create_upstream_request_failed", "Failed to create upstream request")
		return
	}
	httpReq.Header.Set("Authorization", "Bearer "+channel.Key)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-DashScope-Async", "enable")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		logger.Errorf(ctx, "create ali video task failed: %s", err.Error())
		writeVideoError(c, http.StatusBadGateway, "upstream_error", "video_generation_upstream_request_failed", "Upstream video generation request failed")
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var aliResp aliVideoTaskResponse
	if err = json.Unmarshal(body, &aliResp); err != nil {
		writeVideoError(c, http.StatusBadGateway, "upstream_error", "invalid_upstream_response", "Invalid upstream video generation response")
		return
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || aliResp.Output.TaskID == "" {
		message := aliResp.Message
		if aliResp.Output.Message != "" {
			message = aliResp.Output.Message
		}
		if message == "" {
			message = string(body)
		}
		writeVideoError(c, http.StatusBadGateway, "upstream_error", "video_generation_upstream_error", message)
		return
	}

	task := &model.VideoGenerationTask{
		UserId:         c.GetInt(ctxkey.Id),
		TokenId:        c.GetInt(ctxkey.TokenId),
		ChannelId:      channel.Id,
		Provider:       videoProviderAli,
		Model:          req.Model,
		ProviderTaskId: aliResp.Output.TaskID,
		Status:         normalizeAliTaskStatus(aliResp.Output.TaskStatus),
		Quota:          quota,
		RequestBody:    string(payload),
		ResponseBody:   string(body),
	}
	if err = task.Insert(); err != nil {
		writeVideoError(c, http.StatusInternalServerError, "one_api_error", "persist_video_generation_task_failed", "Failed to persist video generation task")
		return
	}

	c.JSON(http.StatusOK, buildVideoTaskResponse(task))
}

func GetVideoGenerationTask(c *gin.Context) {
	task, channel, ok := loadOwnedTask(c)
	if !ok {
		return
	}
	if refreshAliVideoTask(c, task, channel) == nil {
		return
	}
	c.JSON(http.StatusOK, buildVideoTaskResponse(task))
}

func CancelVideoGenerationTask(c *gin.Context) {
	task, channel, ok := loadOwnedTask(c)
	if !ok {
		return
	}
	if task.Provider != videoProviderAli {
		writeVideoError(c, http.StatusBadRequest, "invalid_request_error", "provider_not_supported_for_video_generation", "Current task provider does not support cancel yet")
		return
	}

	baseURL := normalizeBaseURL(channel.GetBaseURL())
	url := fmt.Sprintf("%s/api/v1/tasks/%s/cancel", baseURL, task.ProviderTaskId)
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, url, nil)
	if err != nil {
		writeVideoError(c, http.StatusInternalServerError, "one_api_error", "create_upstream_request_failed", "Failed to create upstream cancel request")
		return
	}
	req.Header.Set("Authorization", "Bearer "+channel.Key)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		writeVideoError(c, http.StatusBadGateway, "upstream_error", "video_generation_cancel_failed", "Upstream video generation cancel request failed")
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		writeVideoError(c, http.StatusBadGateway, "upstream_error", "video_generation_cancel_failed", string(body))
		return
	}

	task.Status = videoStatusCancelled
	task.ResponseBody = string(body)
	task.ErrorCode = ""
	task.ErrorMessage = ""
	if err = task.Update(); err != nil {
		writeVideoError(c, http.StatusInternalServerError, "one_api_error", "persist_video_generation_task_failed", "Failed to update video generation task")
		return
	}
	c.JSON(http.StatusOK, buildVideoTaskResponse(task))
}

func loadOwnedTask(c *gin.Context) (*model.VideoGenerationTask, *model.Channel, bool) {
	task, err := model.GetUserVideoGenerationTaskById(c.Param("id"), c.GetInt(ctxkey.Id))
	if err != nil {
		writeVideoError(c, http.StatusNotFound, "invalid_request_error", "video_generation_task_not_found", "Video generation task not found")
		return nil, nil, false
	}
	channel, err := model.GetChannelById(task.ChannelId, true)
	if err != nil {
		writeVideoError(c, http.StatusInternalServerError, "one_api_error", "channel_not_found", "Task channel not found")
		return nil, nil, false
	}
	return task, channel, true
}

func refreshAliVideoTask(c *gin.Context, task *model.VideoGenerationTask, channel *model.Channel) *model.VideoGenerationTask {
	baseURL := normalizeBaseURL(channel.GetBaseURL())
	url := fmt.Sprintf("%s/api/v1/tasks/%s", baseURL, task.ProviderTaskId)
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, url, nil)
	if err != nil {
		writeVideoError(c, http.StatusInternalServerError, "one_api_error", "create_upstream_request_failed", "Failed to create upstream task request")
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+channel.Key)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		writeVideoError(c, http.StatusBadGateway, "upstream_error", "video_generation_task_fetch_failed", "Upstream video generation task request failed")
		return nil
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var aliResp aliVideoTaskResponse
	if err = json.Unmarshal(body, &aliResp); err != nil {
		writeVideoError(c, http.StatusBadGateway, "upstream_error", "invalid_upstream_response", "Invalid upstream video generation task response")
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := aliResp.Message
		if aliResp.Output.Message != "" {
			message = aliResp.Output.Message
		}
		if message == "" {
			message = string(body)
		}
		writeVideoError(c, http.StatusBadGateway, "upstream_error", "video_generation_task_fetch_failed", message)
		return nil
	}

	task.Status = normalizeAliTaskStatus(aliResp.Output.TaskStatus)
	task.ResponseBody = string(body)
	task.ResultURL = aliResp.Output.VideoURL
	task.ErrorCode = aliResp.Output.Code
	task.ErrorMessage = aliResp.Output.Message
	if aliResp.Code != "" {
		task.ErrorCode = aliResp.Code
	}
	if aliResp.Message != "" && task.ErrorMessage == "" {
		task.ErrorMessage = aliResp.Message
	}
	if err = task.Update(); err != nil {
		writeVideoError(c, http.StatusInternalServerError, "one_api_error", "persist_video_generation_task_failed", "Failed to update video generation task")
		return nil
	}
	if task.Status == videoStatusSucceeded && !task.IsBilled {
		if err = billVideoTask(c, task); err != nil {
			writeVideoError(c, http.StatusInternalServerError, "one_api_error", "video_generation_billing_failed", err.Error())
			return nil
		}
	}
	return task
}

func buildAliVideoGenerationRequest(req VideoGenerationRequest) (*aliVideoGenerationRequest, string, error) {
	aliReq := &aliVideoGenerationRequest{Model: req.Model}
	aliReq.Input.Prompt = req.Prompt
	aliReq.Parameters.PromptExtend = true

	hasImage := req.Image != nil && strings.TrimSpace(*req.Image) != ""
	hasFirst := req.FirstFrameImage != nil && strings.TrimSpace(*req.FirstFrameImage) != ""
	hasLast := req.LastFrameImage != nil && strings.TrimSpace(*req.LastFrameImage) != ""

	switch {
	case hasFirst:
		aliReq.Input.FirstFrameURL = strings.TrimSpace(*req.FirstFrameImage)
		if hasLast {
			aliReq.Input.LastFrameURL = strings.TrimSpace(*req.LastFrameImage)
		}
		aliReq.Parameters.Resolution = normalizeVideoResolution(req.Size)
		if req.Duration > 0 {
			aliReq.Parameters.Duration = req.Duration
		}
		return aliReq, videoEndpointImageToVideo, nil
	case hasImage:
		aliReq.Input.ImgURL = strings.TrimSpace(*req.Image)
		aliReq.Parameters.Resolution = normalizeVideoResolution(req.Size)
		if req.Duration > 0 {
			aliReq.Parameters.Duration = req.Duration
		}
		return aliReq, videoEndpointImageToVideo, nil
	default:
		aliReq.Parameters.Size = normalizeVideoSize(req.Size)
		if req.Duration > 0 {
			aliReq.Parameters.Duration = req.Duration
		}
		return aliReq, videoEndpointTextToVideo, nil
	}
}

func buildVideoTaskResponse(task *model.VideoGenerationTask) videoTaskResponse {
	resp := videoTaskResponse{
		ID:        task.Id,
		Object:    "video.generation.task",
		Status:    task.Status,
		Model:     task.Model,
		Provider:  task.Provider,
		ChannelID: task.ChannelId,
	}
	if task.ResultURL != "" {
		resp.Data = []videoTaskURL{{URL: task.ResultURL}}
	}
	if task.ErrorCode != "" || task.ErrorMessage != "" {
		resp.Error = &videoTaskError{Code: task.ErrorCode, Message: task.ErrorMessage}
	}
	return resp
}

type videoQuotaError struct {
	status  int
	errType string
	code    string
	message string
}

func estimateVideoTaskQuota(c *gin.Context, channel *model.Channel, modelName string) (int64, *videoQuotaError) {
	cfg, err := channel.LoadConfig()
	if err != nil {
		return 0, &videoQuotaError{
			status:  http.StatusInternalServerError,
			errType: "one_api_error",
			code:    "invalid_channel_config",
			message: "Failed to load channel config",
		}
	}
	userId := c.GetInt(ctxkey.Id)
	group := c.GetString(ctxkey.Group)
	if group == "" {
		group, err = model.CacheGetUserGroup(userId)
		if err != nil {
			return 0, &videoQuotaError{
				status:  http.StatusInternalServerError,
				errType: "one_api_error",
				code:    "get_user_group_failed",
				message: "Failed to resolve user group",
			}
		}
	}
	modelRatio := billingratio.GetModelRatioWithOverride(modelName, channel.Type, cfg.ModelRatio)
	groupRatio := billingratio.GetGroupRatio(group)
	channelRatio := billingratio.GetChannelRatio(cfg.ChannelRatio)
	quota := int64(math.Ceil(modelRatio * groupRatio * channelRatio * 1000))
	if modelRatio != 0 && quota <= 0 {
		quota = 1
	}
	userQuota, err := model.CacheGetUserQuota(c.Request.Context(), userId)
	if err != nil {
		return 0, &videoQuotaError{
			status:  http.StatusInternalServerError,
			errType: "one_api_error",
			code:    "get_user_quota_failed",
			message: "Failed to fetch user quota",
		}
	}
	if userQuota-quota < 0 {
		return 0, &videoQuotaError{
			status:  http.StatusForbidden,
			errType: "invalid_request_error",
			code:    "insufficient_user_quota",
			message: "user quota is not enough",
		}
	}
	return quota, nil
}

func billVideoTask(c *gin.Context, task *model.VideoGenerationTask) error {
	if task.Quota <= 0 {
		task.IsBilled = true
		return task.Update()
	}
	if err := model.PostConsumeTokenQuota(task.TokenId, task.Quota); err != nil {
		return err
	}
	if err := model.CacheUpdateUserQuota(c.Request.Context(), task.UserId); err != nil {
		return err
	}
	token, err := model.GetTokenById(task.TokenId)
	if err != nil {
		logger.Error(c.Request.Context(), "get token for video billing failed: "+err.Error())
	}
	tokenName := ""
	if err == nil && token != nil {
		tokenName = token.Name
	}
	model.RecordConsumeLog(c.Request.Context(), &model.Log{
		UserId:           task.UserId,
		ChannelId:        task.ChannelId,
		ModelName:        task.Model,
		TokenName:        tokenName,
		Quota:            int(task.Quota),
		Content:          "视频生成消费",
		PromptTokens:     0,
		CompletionTokens: 0,
	})
	model.UpdateUserUsedQuotaAndRequestCount(task.UserId, task.Quota)
	model.UpdateChannelUsedQuota(task.ChannelId, task.Quota)
	task.IsBilled = true
	return task.Update()
}

func writeVideoError(c *gin.Context, status int, errType string, code string, message string) {
	c.JSON(status, gin.H{
		"error": gin.H{
			"message": helper.MessageWithRequestId(message, c.GetString(helper.RequestIdKey)),
			"type":    errType,
			"code":    code,
		},
	})
}

func normalizeVideoSize(size string) string {
	size = strings.TrimSpace(strings.ToLower(size))
	size = strings.ReplaceAll(size, "x", "*")
	if size == "" {
		return "1280*720"
	}
	return size
}

func normalizeVideoResolution(size string) string {
	size = strings.TrimSpace(strings.ToUpper(size))
	switch size {
	case "", "1280X720", "1280*720", "720P":
		return "720P"
	case "1920X1080", "1920*1080", "1080P":
		return "1080P"
	case "640X480", "640*480", "480P":
		return "480P"
	default:
		return size
	}
}

func normalizeAliTaskStatus(status string) string {
	switch strings.ToUpper(status) {
	case "PENDING":
		return videoStatusQueued
	case "RUNNING":
		return videoStatusRunning
	case "SUCCEEDED":
		return videoStatusSucceeded
	case "FAILED":
		return videoStatusFailed
	case "CANCELED":
		return videoStatusCancelled
	default:
		return videoStatusUnknown
	}
}

func isAliVideoChannel(channelType int) bool {
	return channelType == channeltype.Ali || channelType == channeltype.AliBailian
}

func normalizeBaseURL(baseURL string) string {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return "https://dashscope.aliyuncs.com"
	}
	return strings.TrimRight(baseURL, "/")
}
