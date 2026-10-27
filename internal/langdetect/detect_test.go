package langdetect

import "testing"

func TestDetect(t *testing.T) {
	t.Parallel()
	tests := []struct {
		text string
		code string
	}{
		{"你好，欢迎使用文字转语音。", "zh"},
		{"こんにちは、音声を作成します。", "ja"},
		{"안녕하세요. 음성을 생성합니다.", "ko"},
		{"Hello, welcome to this text to speech tool.", "en"},
		{"Bonjour, merci pour votre aide avec ce texte.", "fr"},
		{"Hallo und danke für diese Sprache.", "de"},
		{"Hola, gracias por este texto en español.", "es"},
		{"Olá, obrigado por este texto em português.", "pt"},
		{"Ciao, grazie per questo testo italiano.", "it"},
		{"Привет, это тест синтеза речи.", "ru"},
		{"مرحبا، هذا اختبار تحويل النص إلى كلام.", "ar"},
		{"नमस्ते, यह वाक् परीक्षण है।", "hi"},
	}
	for _, test := range tests {
		got := Detect(test.text)
		if got.Code != test.code {
			t.Errorf("Detect(%q) = %+v, want %s", test.text, got, test.code)
		}
	}
}

func TestDetectUnknown(t *testing.T) {
	t.Parallel()
	got := Detect("1234 !!!")
	if got.Code != "" || got.Name != "未知" {
		t.Fatalf("unexpected result: %+v", got)
	}
}
