package model

// VideoAspect 视频尺寸比例
type VideoAspect string

const (
	AspectPortrait  VideoAspect = "9:16" // 竖屏 1080x1920
	AspectLandscape VideoAspect = "16:9" // 横屏 1920x1080
)

// VideoConcatMode 素材拼接模式
type VideoConcatMode string

const (
	ConcatRandom     VideoConcatMode = "random"
	ConcatSequential VideoConcatMode = "sequential"
	ConcatReverse    VideoConcatMode = "reverse"
)

// VideoTransitionMode 转场模式
type VideoTransitionMode string

const (
	TransitionNone    VideoTransitionMode = "none"
	TransitionFade    VideoTransitionMode = "fade"
	TransitionSlide   VideoTransitionMode = "slide"
	TransitionDissolve VideoTransitionMode = "dissolve"
)

// VideoParams 视频生成参数
type VideoParams struct {
	// 核心输入
	Subject string `json:"subject"`           // 视频主题
	Script  string `json:"script,omitempty"`  // 自定义文案（留空则自动生成）

	// 语言与风格
	Language        string `json:"language"`          // 语言：zh-CN / en-US
	ParagraphNumber int    `json:"paragraph_number"`  // 文案段落数（1-10）

	// 视频规格
	Aspect          VideoAspect     `json:"aspect"`           // 竖屏/横屏
	ConcatMode      VideoConcatMode `json:"concat_mode"`      // 拼接模式
	TransitionMode  VideoTransitionMode `json:"transition_mode"` // 转场模式
	ClipDuration    int             `json:"clip_duration"`    // 单片段时长（秒）

	// 批量
	VideoCount int `json:"video_count"` // 生成视频数量

	// TTS 配置
	VoiceName string  `json:"voice_name"` // 语音名称
	VoiceRate float64 `json:"voice_rate"` // 语音速率（-50% ~ +100%）

	// 字幕配置
	SubtitleEnabled  bool   `json:"subtitle_enabled"`
	SubtitleFont     string `json:"subtitle_font"`
	SubtitleSize     int    `json:"subtitle_size"`
	SubtitleColor    string `json:"subtitle_color"`
	SubtitlePosition string `json:"subtitle_position"` // top / center / bottom
	SubtitleStroke   bool   `json:"subtitle_stroke"`

	// 背景音乐
	BGMEnabled bool    `json:"bgm_enabled"`
	BGMFile    string  `json:"bgm_file,omitempty"` // 指定 BGM 文件（留空随机）
	BGMVolume  float64 `json:"bgm_volume"`         // BGM 音量（0.0-1.0）

	// 素材来源覆盖（空则使用全局配置）
	MaterialSource string `json:"material_source,omitempty"`
	LocalMaterialDir string `json:"local_material_dir,omitempty"`
}

// Defaults 填充默认值
func (p *VideoParams) Defaults() {
	if p.Language == "" {
		p.Language = "zh-CN"
	}
	if p.ParagraphNumber <= 0 || p.ParagraphNumber > 10 {
		p.ParagraphNumber = 5
	}
	if p.Aspect == "" {
		p.Aspect = AspectPortrait
	}
	if p.ConcatMode == "" {
		p.ConcatMode = ConcatRandom
	}
	if p.TransitionMode == "" {
		p.TransitionMode = TransitionFade
	}
	if p.ClipDuration <= 0 {
		p.ClipDuration = 5
	}
	if p.VideoCount <= 0 {
		p.VideoCount = 1
	}
	if p.VoiceName == "" {
		p.VoiceName = "zh-CN-XiaoyiNeural"
	}
	if p.VoiceRate == 0 {
		p.VoiceRate = 0
	}
	if p.SubtitleSize == 0 {
		p.SubtitleSize = 36
	}
	if p.SubtitleColor == "" {
		p.SubtitleColor = "#FFFFFF"
	}
	if p.SubtitlePosition == "" {
		p.SubtitlePosition = "bottom"
	}
	if p.SubtitleFont == "" {
		p.SubtitleFont = "Arial"
	}
	if p.BGMVolume == 0 {
		p.BGMVolume = 0.2
	}
}

// Width 返回视频宽度（像素）
func (p *VideoParams) Width() int {
	if p.Aspect == AspectLandscape {
		return 1920
	}
	return 1080
}

// Height 返回视频高度（像素）
func (p *VideoParams) Height() int {
	if p.Aspect == AspectLandscape {
		return 1080
	}
	return 1920
}
