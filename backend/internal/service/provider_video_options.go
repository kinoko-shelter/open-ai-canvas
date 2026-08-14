package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var grokVideoResolutionModelPattern = regexp.MustCompile(`(?i)^(grok-imagine-video-[\w.-]+)-(480p|720p|1080p)$`)

func isPublicMediaURL(value string) bool {
	lower := strings.ToLower(value)
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

func isSeedanceVideoConfig(config providerConfig) bool {
	model := strings.ToLower(config.Model)
	return strings.Contains(model, "seedance") || strings.Contains(model, "doubao-seedance") || isArkPlanVideoConfig(config)
}

func isGrokVideoConfig(config providerConfig) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(config.Model)), "grok")
}

func grokVideoResolutionProfile(model string) (string, string) {
	modelName := strings.TrimSpace(model)
	match := grokVideoResolutionModelPattern.FindStringSubmatch(modelName)
	if len(match) != 3 {
		return modelName, ""
	}
	return match[1], strings.ToLower(match[2])
}

func isArkPlanVideoConfig(config providerConfig) bool {
	return strings.Contains(strings.ToLower(config.BaseURL), "/api/plan/v3")
}

func normalizeImageQuality(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1k":
		return "low"
	case "2k":
		return "medium"
	case "4k":
		return "high"
	default:
		return value
	}
}

func imageParameterSupported(profile *ImageCapabilityConfig, parameter string) bool {
	if profile == nil {
		return true
	}
	if parameter == "response_format" {
		return profile.ResponseFormat.Supported
	}
	return profile.OutputFormat.Supported
}

func imageQualitySupported(profile *ImageCapabilityConfig) bool {
	return profile == nil || profile.Quality.Supported
}

func imageTransparentBackgroundSupported(profile *ImageCapabilityConfig) bool {
	return profile == nil || profile.TransparentBackground.Supported
}

func imageSizeParameter(profile *ImageCapabilityConfig, modelName string, quality string, value string) (string, string, error) {
	if isGPTImage2Model(modelName) {
		resolved, err := resolveGPTImage2Size(quality, value)
		return "size", resolved, err
	}
	if profile == nil {
		return "size", normalizePixelSize(value), nil
	}
	value = strings.TrimSpace(value)
	if value == "" || value == "auto" {
		value = strings.TrimSpace(profile.Size.Default)
	}
	switch profile.Size.Parameter {
	case "size":
		return "size", normalizePixelSize(value), nil
	case "aspect_ratio":
		return "aspect_ratio", normalizeImageAspectRatio(value), nil
	default:
		return "", "", nil
	}
}

func resolveGPTImage2Size(quality string, value string) (string, error) {
	// 比例预设按档位转为标准像素，合法的显式 WxH 必须原样透传给上游。
	value = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(value, "×", "x")))
	if value == "" || value == "auto" {
		return "auto", nil
	}
	if strings.Contains(value, "x") {
		parts := strings.Split(value, "x")
		if len(parts) != 2 {
			return "", errors.New("gpt-image-2 尺寸格式无效")
		}
		width, widthErr := strconv.Atoi(parts[0])
		height, heightErr := strconv.Atoi(parts[1])
		if widthErr != nil || heightErr != nil || width <= 0 || height <= 0 {
			return "", errors.New("gpt-image-2 尺寸必须是正整数")
		}
		if width%16 != 0 || height%16 != 0 {
			return "", errors.New("gpt-image-2 自定义宽高必须是 16 的倍数")
		}
		return fmt.Sprintf("%dx%d", width, height), nil
	}
	if !gptImage2RatioSupported(value) {
		return "", errors.New("gpt-image-2 不支持该宽高比")
	}
	parts := strings.Split(value, ":")
	width, _ := strconv.Atoi(parts[0])
	height, _ := strconv.Atoi(parts[1])
	tier := gptImage2Tier(quality)
	if width == height {
		edge := map[string]int{"1k": 1024, "2k": 2048, "4k": 2480}[tier]
		return fmt.Sprintf("%dx%d", edge, edge), nil
	}
	landscape := width > height
	longEdge := gptImage2LongEdge(tier, landscape)
	shortRatio, longRatio := width, height
	if landscape {
		shortRatio, longRatio = height, width
	}
	shortEdge := ((longEdge*shortRatio + longRatio*16 - 1) / (longRatio * 16)) * 16
	if landscape {
		return fmt.Sprintf("%dx%d", longEdge, shortEdge), nil
	}
	return fmt.Sprintf("%dx%d", shortEdge, longEdge), nil
}

func gptImage2RatioSupported(value string) bool {
	for _, ratio := range []string{"1:1", "3:2", "2:3", "4:3", "3:4", "5:4", "4:5", "16:9", "9:16", "21:9"} {
		if value == ratio {
			return true
		}
	}
	return false
}

func gptImage2Tier(quality string) string {
	switch strings.ToLower(strings.TrimSpace(quality)) {
	case "4k", "high":
		return "4k"
	case "2k", "medium":
		return "2k"
	default:
		return "1k"
	}
}

func gptImage2LongEdge(tier string, landscape bool) int {
	switch tier {
	case "4k":
		if landscape {
			return 3312
		}
		return 3328
	case "2k":
		if landscape {
			return 2048
		}
		return 2560
	default:
		return 1280
	}
}

func normalizeImageAspectRatio(value string) string {
	value = strings.TrimSpace(strings.ToLower(strings.ReplaceAll(value, "×", "x")))
	if strings.Contains(value, ":") {
		return value
	}
	parts := strings.Split(value, "x")
	if len(parts) != 2 {
		return ""
	}
	width, widthErr := strconv.Atoi(parts[0])
	height, heightErr := strconv.Atoi(parts[1])
	if widthErr != nil || heightErr != nil || width <= 0 || height <= 0 {
		return ""
	}
	divisor := imageDimensionGCD(width, height)
	return strconv.Itoa(width/divisor) + ":" + strconv.Itoa(height/divisor)
}

func imageDimensionGCD(left int, right int) int {
	for right != 0 {
		left, right = right, left%right
	}
	if left < 1 {
		return 1
	}
	return left
}

func normalizePixelSize(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "auto" {
		return ""
	}
	// 画布按比例保存常用预设；图片接口只接受像素尺寸，必须在请求边界完成转换。
	switch value {
	case "1:1":
		return "1024x1024"
	case "3:2":
		return "1536x1024"
	case "2:3":
		return "1024x1536"
	case "4:3":
		return "1360x1024"
	case "3:4":
		return "1024x1360"
	case "16:9":
		return "1824x1024"
	case "9:16":
		return "1024x1824"
	case "21:9":
		return "2352x1008"
	}
	if strings.Contains(value, "x") {
		return value
	}
	return ""
}

func normalizeVideoSize(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "auto" {
		return ""
	}
	if strings.Contains(value, "x") {
		return value
	}
	if value == "9:16" || value == "2:3" || value == "3:4" {
		return "720x1280"
	}
	return "1280x720"
}

func normalizeVideoResolution(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "auto" || value == "medium" || value == "high" {
		return "720p"
	}
	if value == "low" {
		return "480p"
	}
	if strings.EqualFold(value, "4k") {
		return "2160p"
	}
	if strings.HasSuffix(value, "p") {
		return value
	}
	return value + "p"
}

func normalizeXAIVideoDuration(value string) int {
	duration, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || duration <= 0 {
		return 6
	}
	return duration
}

func normalizeXAIVideoResolution(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "480", "480p", "low":
		return "480p"
	case "1080", "1080p":
		return "1080p"
	case "2160", "2160p", "4k":
		return "2160p"
	default:
		return "720p"
	}
}

func normalizeXAIVideoAspectRatio(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	allowed := map[string]bool{
		"1:1": true, "16:9": true, "9:16": true, "4:3": true,
		"3:4": true, "3:2": true, "2:3": true,
	}
	if allowed[value] {
		return value
	}
	parts := strings.Split(value, "x")
	if len(parts) != 2 {
		return "16:9"
	}
	width, widthErr := strconv.Atoi(strings.TrimSpace(parts[0]))
	height, heightErr := strconv.Atoi(strings.TrimSpace(parts[1]))
	if widthErr != nil || heightErr != nil || width <= 0 || height <= 0 {
		return "16:9"
	}
	ratio := float64(width) / float64(height)
	candidates := []struct {
		name  string
		ratio float64
	}{
		{name: "1:1", ratio: 1},
		{name: "16:9", ratio: 16.0 / 9},
		{name: "9:16", ratio: 9.0 / 16},
		{name: "4:3", ratio: 4.0 / 3},
		{name: "3:4", ratio: 3.0 / 4},
		{name: "3:2", ratio: 3.0 / 2},
		{name: "2:3", ratio: 2.0 / 3},
	}
	bestName := "16:9"
	bestDifference := 2.0
	for _, candidate := range candidates {
		difference := ratio - candidate.ratio
		if difference < 0 {
			difference = -difference
		}
		if difference < bestDifference {
			bestName = candidate.name
			bestDifference = difference
		}
	}
	return bestName
}

func normalizeSeedanceDuration(value string) int {
	if strings.TrimSpace(value) == "-1" {
		return -1
	}
	seconds, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || seconds <= 0 {
		return 5
	}
	return seconds
}

func normalizeSeedanceVideosDuration(value string) int {
	return normalizeSeedanceDuration(value)
}

func normalizeSeedanceRatio(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "auto" || value == "adaptive" {
		return "adaptive"
	}
	switch value {
	case "16:9", "9:16", "1:1", "4:3", "3:4", "21:9":
		return value
	default:
		return "adaptive"
	}
}

func normalizeSeedanceVideosRatio(value string) string {
	ratio := normalizeSeedanceRatio(value)
	if ratio == "adaptive" {
		return "16:9"
	}
	return ratio
}

func normalizeSeedanceResolution(value string, model string) string {
	resolution := strings.TrimSuffix(strings.TrimSpace(value), "p")
	if strings.EqualFold(resolution, "4k") {
		resolution = "2160"
	}
	switch resolution {
	case "480", "720", "1080", "2160":
	default:
		if value == "low" {
			resolution = "480"
		} else {
			resolution = "720"
		}
	}
	if strings.Contains(strings.ToLower(model), "fast") && (resolution == "1080" || resolution == "2160") {
		resolution = "720"
	}
	return resolution + "p"
}

func parseBool(value string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true":
		return true
	case "false":
		return false
	default:
		return fallback
	}
}

func parseFloat(value string, fallback float64) float64 {
	number, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || number == 0 {
		return fallback
	}
	return number
}

func sleepContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
