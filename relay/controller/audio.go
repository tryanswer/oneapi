package controller

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/client"
	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/common/logger"
	"github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/billing"
	billingratio "github.com/songquanpeng/one-api/relay/billing/ratio"
	"github.com/songquanpeng/one-api/relay/channeltype"
	"github.com/songquanpeng/one-api/relay/meta"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

const (
	audioCompatModelPrefix = "qwen3-asr"
	maxAudioCompatBytes    = 30 * 1024 * 1024
)

type audioCompatPayload struct {
	Model          string
	Language       string
	Prompt         string
	ResponseFormat string
	DataURI        string
}

type slimChatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content interface{} `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func RelayAudioHelper(c *gin.Context, relayMode int) *relaymodel.ErrorWithStatusCode {
	ctx := c.Request.Context()
	meta := meta.GetByContext(c)
	audioModel := "qwen3-asr-flash"

	tokenId := c.GetInt(ctxkey.TokenId)
	channelType := c.GetInt(ctxkey.Channel)
	channelId := c.GetInt(ctxkey.ChannelId)
	userId := c.GetInt(ctxkey.Id)
	group := c.GetString(ctxkey.Group)
	tokenName := c.GetString(ctxkey.TokenName)

	var ttsRequest openai.TextToSpeechRequest
	if relayMode == relaymode.AudioSpeech {
		// Read JSON
		err := common.UnmarshalBodyReusable(c, &ttsRequest)
		// Check if JSON is valid
		if err != nil {
			return openai.ErrorWrapper(err, "invalid_json", http.StatusBadRequest)
		}
		audioModel = ttsRequest.Model
		// Check if text is too long 4096
		if len(ttsRequest.Input) > 4096 {
			return openai.ErrorWrapper(errors.New("input is too long (over 4096 characters)"), "text_too_long", http.StatusBadRequest)
		}
	}
	if relayMode == relaymode.AudioTranscription || relayMode == relaymode.AudioTranslation {
		// Parse multipart form to get model name. Reset body for later proxying.
		bodyBytes, err := io.ReadAll(c.Request.Body)
		if err != nil {
			return openai.ErrorWrapper(err, "read_request_body_failed", http.StatusInternalServerError)
		}
		c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		_ = c.Request.ParseMultipartForm(32 << 20)
		if modelName := c.PostForm("model"); modelName != "" {
			audioModel = modelName
		}
		formKeys := make([]string, 0)
		for key := range c.Request.MultipartForm.Value {
			formKeys = append(formKeys, key)
		}
		logger.Info(ctx, fmt.Sprintf("audio request: method=%s path=%s content-type=%s form_keys=%s model=%s",
			c.Request.Method,
			c.Request.URL.Path,
			c.Request.Header.Get("Content-Type"),
			strings.Join(formKeys, ","),
			audioModel,
		))
		c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	}
	logger.Info(ctx, fmt.Sprintf("audio request model: %s", audioModel))

	modelRatio := billingratio.GetModelRatioWithOverride(audioModel, channelType, meta.Config.ModelRatio)
	groupRatio := billingratio.GetGroupRatio(group)
	channelRatio := billingratio.GetChannelRatio(meta.Config.ChannelRatio)
	ratio := modelRatio * groupRatio * channelRatio
	var quota int64
	var preConsumedQuota int64
	switch relayMode {
	case relaymode.AudioSpeech:
		preConsumedQuota = int64(float64(len(ttsRequest.Input)) * ratio)
		quota = preConsumedQuota
	default:
		preConsumedQuota = int64(float64(config.PreConsumedQuota) * ratio)
	}
	userQuota, err := model.CacheGetUserQuota(ctx, userId)
	if err != nil {
		return openai.ErrorWrapper(err, "get_user_quota_failed", http.StatusInternalServerError)
	}

	// Check if user quota is enough
	if userQuota-preConsumedQuota < 0 {
		return openai.ErrorWrapper(errors.New("user quota is not enough"), "insufficient_user_quota", http.StatusForbidden)
	}
	err = model.CacheDecreaseUserQuota(userId, preConsumedQuota)
	if err != nil {
		return openai.ErrorWrapper(err, "decrease_user_quota_failed", http.StatusInternalServerError)
	}
	if userQuota > 100*preConsumedQuota {
		// in this case, we do not pre-consume quota
		// because the user has enough quota
		preConsumedQuota = 0
	}
	if preConsumedQuota > 0 {
		err := model.PreConsumeTokenQuota(tokenId, preConsumedQuota)
		if err != nil {
			return openai.ErrorWrapper(err, "pre_consume_token_quota_failed", http.StatusForbidden)
		}
	}
	succeed := false
	defer func() {
		if succeed {
			return
		}
		if preConsumedQuota > 0 {
			// we need to roll back the pre-consumed quota
			defer func(ctx context.Context) {
				go func() {
					// negative means add quota back for token & user
					err := model.PostConsumeTokenQuota(tokenId, -preConsumedQuota)
					if err != nil {
						logger.Error(ctx, fmt.Sprintf("error rollback pre-consumed quota: %s", err.Error()))
					}
				}()
			}(c.Request.Context())
		}
	}()

	// map model name
	modelMapping := c.GetStringMapString(ctxkey.ModelMapping)
	if modelMapping != nil && modelMapping[audioModel] != "" {
		audioModel = modelMapping[audioModel]
	}
	logger.Info(ctx, fmt.Sprintf("audio request model after mapping: %s", audioModel))

	compatPayload, compatErr := maybeBuildAudioCompatPayload(
		c,
		relayMode,
		audioModel,
	)
	if compatErr != nil {
		return openai.ErrorWrapper(compatErr, "audio_compat_parse_failed", http.StatusBadRequest)
	}

	baseURL := channeltype.ChannelBaseURLs[channelType]
	requestURL := c.Request.URL.String()
	if c.GetString(ctxkey.BaseURL) != "" {
		baseURL = c.GetString(ctxkey.BaseURL)
	}

	fullRequestURL := openai.GetFullRequestURL(baseURL, requestURL, channelType)
	if channelType == channeltype.Azure {
		apiVersion := meta.Config.APIVersion
		if relayMode == relaymode.AudioTranscription {
			// https://learn.microsoft.com/en-us/azure/ai-services/openai/whisper-quickstart?tabs=command-line#rest-api
			fullRequestURL = fmt.Sprintf("%s/openai/deployments/%s/audio/transcriptions?api-version=%s", baseURL, audioModel, apiVersion)
		} else if relayMode == relaymode.AudioSpeech {
			// https://learn.microsoft.com/en-us/azure/ai-services/openai/text-to-speech-quickstart?tabs=command-line#rest-api
			fullRequestURL = fmt.Sprintf("%s/openai/deployments/%s/audio/speech?api-version=%s", baseURL, audioModel, apiVersion)
		}
	}

	requestBody := &bytes.Buffer{}
	if compatPayload != nil {
		chatBody, err := buildAudioCompatChatBody(audioModel, compatPayload)
		if err != nil {
			return openai.ErrorWrapper(err, "audio_compat_build_failed", http.StatusInternalServerError)
		}
		requestBody = bytes.NewBuffer(chatBody)
		logger.Info(ctx, fmt.Sprintf("audio compat: using /chat/completions for model=%s", audioModel))
	} else {
		_, err = io.Copy(requestBody, c.Request.Body)
		if err != nil {
			return openai.ErrorWrapper(err, "new_request_body_failed", http.StatusInternalServerError)
		}
		c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody.Bytes()))
	}
	responseFormat := c.DefaultPostForm("response_format", "json")

	if compatPayload != nil {
		fullRequestURL = openai.GetFullRequestURL(baseURL, "/v1/chat/completions", channelType)
	}
	req, err := http.NewRequest(c.Request.Method, fullRequestURL, requestBody)
	if err != nil {
		return openai.ErrorWrapper(err, "new_request_failed", http.StatusInternalServerError)
	}

	if (relayMode == relaymode.AudioTranscription || relayMode == relaymode.AudioSpeech) && channelType == channeltype.Azure && compatPayload == nil {
		// https://learn.microsoft.com/en-us/azure/ai-services/openai/whisper-quickstart?tabs=command-line#rest-api
		apiKey := c.Request.Header.Get("Authorization")
		apiKey = strings.TrimPrefix(apiKey, "Bearer ")
		req.Header.Set("api-key", apiKey)
		req.ContentLength = c.Request.ContentLength
	} else {
		req.Header.Set("Authorization", c.Request.Header.Get("Authorization"))
	}
	if compatPayload != nil {
		req.Header.Set("Content-Type", "application/json")
	} else {
		req.Header.Set("Content-Type", c.Request.Header.Get("Content-Type"))
	}
	req.Header.Set("Accept", c.Request.Header.Get("Accept"))

	resp, err := client.HTTPClient.Do(req)
	if err != nil {
		return openai.ErrorWrapper(err, "do_request_failed", http.StatusInternalServerError)
	}

	err = req.Body.Close()
	if err != nil {
		return openai.ErrorWrapper(err, "close_request_body_failed", http.StatusInternalServerError)
	}
	err = c.Request.Body.Close()
	if err != nil {
		return openai.ErrorWrapper(err, "close_request_body_failed", http.StatusInternalServerError)
	}

	if relayMode != relaymode.AudioSpeech {
		responseBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return openai.ErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		}
		err = resp.Body.Close()
		if err != nil {
			return openai.ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError)
		}

		var openAIErr openai.SlimTextResponse
		if err = json.Unmarshal(responseBody, &openAIErr); err == nil {
			if openAIErr.Error.Message != "" {
				return openai.ErrorWrapper(fmt.Errorf("type %s, code %v, message %s", openAIErr.Error.Type, openAIErr.Error.Code, openAIErr.Error.Message), "request_error", http.StatusInternalServerError)
			}
		}

		var text string
		if compatPayload != nil {
			text, err = extractTextFromChatCompletion(responseBody)
			if err == nil {
				responseBody = buildTranscriptionResponseBody(text, responseFormat)
			}
		} else {
			switch responseFormat {
			case "json":
				text, err = getTextFromJSON(responseBody)
			case "text":
				text, err = getTextFromText(responseBody)
			case "srt":
				text, err = getTextFromSRT(responseBody)
			case "verbose_json":
				text, err = getTextFromVerboseJSON(responseBody)
			case "vtt":
				text, err = getTextFromVTT(responseBody)
			default:
				return openai.ErrorWrapper(errors.New("unexpected_response_format"), "unexpected_response_format", http.StatusInternalServerError)
			}
		}
		if err != nil {
			return openai.ErrorWrapper(err, "get_text_from_body_err", http.StatusInternalServerError)
		}
		quota = int64(openai.CountTokenText(text, audioModel))
		resp.Body = io.NopCloser(bytes.NewBuffer(responseBody))
		if compatPayload != nil {
			resp.Header.Set("Content-Type", "application/json")
		}
	}
	if resp.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(resp.Body)
		if len(responseBody) > 0 {
			logger.Info(ctx, fmt.Sprintf("audio upstream error: status=%d body=%s", resp.StatusCode, string(responseBody)))
		} else {
			logger.Info(ctx, fmt.Sprintf("audio upstream error: status=%d body=<empty>", resp.StatusCode))
		}
		resp.Body = io.NopCloser(bytes.NewBuffer(responseBody))
		return RelayErrorHandler(resp)
	}
	succeed = true
	quotaDelta := quota - preConsumedQuota
	defer func(ctx context.Context) {
		go billing.PostConsumeQuota(ctx, tokenId, quotaDelta, quota, userId, channelId, modelRatio, groupRatio, audioModel, tokenName)
	}(c.Request.Context())

	for k, v := range resp.Header {
		c.Writer.Header().Set(k, v[0])
	}
	c.Writer.WriteHeader(resp.StatusCode)

	_, err = io.Copy(c.Writer, resp.Body)
	if err != nil {
		return openai.ErrorWrapper(err, "copy_response_body_failed", http.StatusInternalServerError)
	}
	err = resp.Body.Close()
	if err != nil {
		return openai.ErrorWrapper(err, "close_response_body_failed", http.StatusInternalServerError)
	}
	return nil
}

func maybeBuildAudioCompatPayload(
	c *gin.Context,
	relayMode int,
	audioModel string,
) (*audioCompatPayload, error) {
	if relayMode != relaymode.AudioTranscription {
		return nil, nil
	}
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(audioModel)), audioCompatModelPrefix) {
		return nil, nil
	}
	contentType := c.Request.Header.Get("Content-Type")
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(mediaType, "multipart/form-data") {
		return nil, nil
	}
	boundary := params["boundary"]
	if boundary == "" {
		return nil, errors.New("missing multipart boundary")
	}
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return nil, err
	}
	c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	reader := multipart.NewReader(bytes.NewReader(bodyBytes), boundary)
	payload := &audioCompatPayload{Model: audioModel, ResponseFormat: "json"}
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		name := part.FormName()
		if name == "" {
			continue
		}
		switch name {
		case "model":
			if b, err := io.ReadAll(part); err == nil {
				if v := strings.TrimSpace(string(b)); v != "" {
					payload.Model = v
				}
			}
		case "language":
			if b, err := io.ReadAll(part); err == nil {
				payload.Language = strings.TrimSpace(string(b))
			}
		case "prompt":
			if b, err := io.ReadAll(part); err == nil {
				payload.Prompt = strings.TrimSpace(string(b))
			}
		case "response_format":
			if b, err := io.ReadAll(part); err == nil {
				if v := strings.TrimSpace(string(b)); v != "" {
					payload.ResponseFormat = v
				}
			}
		case "file":
			data, err := io.ReadAll(part)
			if err != nil {
				return nil, err
			}
			if len(data) == 0 {
				return nil, errors.New("empty audio file")
			}
			if len(data) > maxAudioCompatBytes {
				return nil, fmt.Errorf("audio file too large: %d bytes", len(data))
			}
			ext := strings.ToLower(filepath.Ext(part.FileName()))
			audioMime := "audio/ogg"
			switch ext {
			case ".mp3":
				audioMime = "audio/mpeg"
			case ".wav":
				audioMime = "audio/wav"
			case ".m4a", ".mp4":
				audioMime = "audio/mp4"
			case ".opus":
				audioMime = "audio/opus"
			case ".oga", ".ogg":
				audioMime = "audio/ogg"
			}
			payload.DataURI = fmt.Sprintf("data:%s;base64,%s", audioMime, base64.StdEncoding.EncodeToString(data))
		}
	}
	if payload.DataURI == "" {
		return nil, errors.New("audio file missing in multipart form")
	}
	return payload, nil
}

func buildAudioCompatChatBody(model string, payload *audioCompatPayload) ([]byte, error) {
	body := map[string]interface{}{
		"model":  model,
		"stream": false,
		"messages": []map[string]interface{}{
			{
				"role": "user",
				"content": []map[string]interface{}{
					{
						"type": "input_audio",
						"input_audio": map[string]interface{}{
							"data": payload.DataURI,
						},
					},
				},
			},
		},
		"asr_options": map[string]interface{}{
			"enable_itn": false,
		},
	}
	if payload.Prompt != "" {
		body["prompt"] = payload.Prompt
	}
	if payload.Language != "" {
		body["language"] = payload.Language
	}
	return json.Marshal(body)
}

func extractTextFromChatCompletion(body []byte) (string, error) {
	var resp slimChatCompletionResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", errors.New("chat completion missing choices")
	}
	content := resp.Choices[0].Message.Content
	switch typed := content.(type) {
	case string:
		return strings.TrimSpace(typed), nil
	case []interface{}:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			obj, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			if txt, ok := obj["text"].(string); ok && strings.TrimSpace(txt) != "" {
				parts = append(parts, strings.TrimSpace(txt))
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "\n"), nil
		}
	}
	return "", errors.New("chat completion missing transcription text")
}

func buildTranscriptionResponseBody(text string, responseFormat string) []byte {
	trimmed := strings.TrimSpace(text)
	switch responseFormat {
	case "text":
		return []byte(trimmed)
	case "verbose_json":
		payload, _ := json.Marshal(map[string]interface{}{
			"text": trimmed,
		})
		return payload
	default:
		payload, _ := json.Marshal(map[string]interface{}{
			"text": trimmed,
		})
		return payload
	}
}

func getTextFromVTT(body []byte) (string, error) {
	return getTextFromSRT(body)
}

func getTextFromVerboseJSON(body []byte) (string, error) {
	var whisperResponse openai.WhisperVerboseJSONResponse
	if err := json.Unmarshal(body, &whisperResponse); err != nil {
		return "", fmt.Errorf("unmarshal_response_body_failed err :%w", err)
	}
	return whisperResponse.Text, nil
}

func getTextFromSRT(body []byte) (string, error) {
	scanner := bufio.NewScanner(strings.NewReader(string(body)))
	var builder strings.Builder
	var textLine bool
	for scanner.Scan() {
		line := scanner.Text()
		if textLine {
			builder.WriteString(line)
			textLine = false
			continue
		} else if strings.Contains(line, "-->") {
			textLine = true
			continue
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return builder.String(), nil
}

func getTextFromText(body []byte) (string, error) {
	return strings.TrimSuffix(string(body), "\n"), nil
}

func getTextFromJSON(body []byte) (string, error) {
	var whisperResponse openai.WhisperJSONResponse
	if err := json.Unmarshal(body, &whisperResponse); err != nil {
		return "", fmt.Errorf("unmarshal_response_body_failed err :%w", err)
	}
	return whisperResponse.Text, nil
}
