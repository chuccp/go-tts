(() => {
  "use strict";

  const translations = {
    zh: {
      "page.title": "VoxLab · 多引擎文字转语音",
      "brand.tagline": "多引擎语音工坊",
      "ui.language": "界面语言",
      "language.zh": "中文",
      "language.en": "English",
      "language.ja": "日本語",
      "language.ko": "한국어",
      "language.fr": "Français",
      "language.de": "Deutsch",
      "language.es": "Español",
      "language.pt": "Português",
      "language.it": "Italiano",
      "language.ru": "Русский",
      "language.ar": "العربية",
      "language.hi": "हिन्दी",
      "hero.title": "多引擎文字转语音",
      "input.title": "输入文本",
      "input.try": "试试：",
      "input.placeholder": "在这里输入或粘贴文字……\n\n例如：欢迎使用 VoxLab。今天，我们把文字变成声音。",
      "sample.zh": "中文叙事",
      "sample.en": "English",
      "sample.quote": "短句",
      "engine.title": "选择引擎",
      "engine.hint": "本地模型需要预先下载",
      "engine.edge.description": "在线 · 400+ 音色 · 90+ 语言",
      "engine.kokoro.description": "本地 ONNX · 82M · 中文与英文音色",
      "engine.kitten.description": "本地 ONNX · 轻量快速 · 8 种音色",
      "engine.available": "可用",
      "engine.unavailable": "未就绪",
      "language.label": "语言",
      "role.label": "角色",
      "speed.label": "语速",
      "button.generate": "生成语音",
      "button.generating": "正在生成",
      "button.generatingSeconds": "正在生成 {seconds}s",
      "result.kicker": "合成结果",
      "result.placeholder": "生成完成后，\n可在此试听和下载音频。",
      "player.kicker": "音频输出",
      "player.waiting": "等待生成",
      "player.waitingHint": "选择文本和音色后点击生成",
      "player.download": "下载音频",
      "player.generatedDetail": "{speed}× 语速 · {size} KB",
      "steps.selectEngine": "选择引擎",
      "steps.tuneVoice": "调节角色",
      "steps.generate": "生成播放",
      "footer.left": "VoxLab · Go + go-web-frame",
      "footer.right": "Kokoro 与 Kitten 在本地推理；Edge TTS 需要网络连接",
      "detect.detecting": "语言检测中…",
      "detect.detected": "检测到：{name} · {confidence}%",
      "detect.unknown": "语言：未识别",
      "detect.failed": "语言检测失败",
      "warning.unsupported": "检测到{language}，{engine} 当前没有该语言的角色。",
      "warning.mismatch": "检测到{detected}，当前角色语言为{selected}，请确认选择。",
      "role.empty": "没有可用角色",
      "voice.syncing": "正在同步音色列表…",
      "voice.count": "{count} 个{language}角色",
      "voice.fallback": "在线音色暂不可用，已使用内置列表：{warning}",
      "voice.failed": "音色同步失败，可继续使用当前列表",
      "gender.female": "女声",
      "gender.male": "男声",
      "error.textRequired": "请先输入要转换的文字。",
      "error.engineUnavailable": "所选引擎当前不可用。",
      "error.voiceSyncing": "音色列表正在同步，请稍后再试。",
      "error.emptyAudio": "服务返回了空音频。",
      "error.synthesis": "合成失败（HTTP {status}）",
      "error.generic": "合成失败，请稍后重试。",
      "error.playback": "浏览器无法播放这段音频，请尝试下载后播放。",
      "captcha.kicker": "SECURITY CHECK",
      "captcha.title": "安全验证",
      "captcha.instruction": "请依次点击下面三个图标：",
      "captcha.remaining": "还需点击 {count} 个图标",
      "captcha.verifying": "正在验证…",
      "captcha.success": "验证成功",
      "captcha.failed": "验证未通过，请重新点击",
      "captcha.clear": "清空",
      "captcha.undo": "撤销",
      "captcha.refresh": "刷新",
      "captcha.close": "关闭验证",
      "captcha.imageAlt": "安全验证图片，包含三个图形图标",
      "captcha.targetAlt": "第 {index} 个目标图标",
      "captcha.loadFailed": "验证图片加载失败，请稍后重试。",
      "captcha.expired": "验证已过期，请重新点击图标。"
    },
    en: {
      "page.title": "VoxLab · Multi-engine Text to Speech",
      "brand.tagline": "Multi-engine TTS workspace",
      "ui.language": "Interface language",
      "language.zh": "Chinese",
      "language.en": "English",
      "language.ja": "Japanese",
      "language.ko": "Korean",
      "language.fr": "French",
      "language.de": "German",
      "language.es": "Spanish",
      "language.pt": "Portuguese",
      "language.it": "Italian",
      "language.ru": "Russian",
      "language.ar": "Arabic",
      "language.hi": "Hindi",
      "hero.title": "Multi-engine Text to Speech",
      "input.title": "Text input",
      "input.try": "Try:",
      "input.placeholder": "Enter or paste text here…\n\nExample: Welcome to VoxLab. Let's turn this text into speech.",
      "sample.zh": "Chinese",
      "sample.en": "English",
      "sample.quote": "Short",
      "engine.title": "Engine",
      "engine.hint": "Local models must be installed",
      "engine.edge.description": "Online · 400+ voices · 90+ languages",
      "engine.kokoro.description": "Local ONNX · 82M · Chinese and English voices",
      "engine.kitten.description": "Local ONNX · lightweight · 8 voices",
      "engine.available": "Ready",
      "engine.unavailable": "Unavailable",
      "language.label": "Language",
      "role.label": "Role",
      "speed.label": "Speed",
      "button.generate": "Generate speech",
      "button.generating": "Generating",
      "button.generatingSeconds": "Generating {seconds}s",
      "result.kicker": "Output",
      "result.placeholder": "Generated audio can be previewed\nand downloaded here.",
      "player.kicker": "Audio output",
      "player.waiting": "Waiting to generate",
      "player.waitingHint": "Choose text and a role, then generate",
      "player.download": "Download audio",
      "player.generatedDetail": "{speed}× speed · {size} KB",
      "steps.selectEngine": "Select engine",
      "steps.tuneVoice": "Choose role",
      "steps.generate": "Generate & play",
      "footer.left": "VoxLab · Go + go-web-frame",
      "footer.right": "Kokoro and Kitten run locally; Edge TTS requires network access",
      "detect.detecting": "Detecting language…",
      "detect.detected": "Detected: {name} · {confidence}%",
      "detect.unknown": "Language: unknown",
      "detect.failed": "Language detection failed",
      "warning.unsupported": "Detected {language}, but {engine} has no roles for that language.",
      "warning.mismatch": "Detected {detected}, but the selected role language is {selected}.",
      "role.empty": "No roles available",
      "voice.syncing": "Syncing voices…",
      "voice.count": "{count} {language} roles",
      "voice.fallback": "Online voices are unavailable; using the built-in list: {warning}",
      "voice.failed": "Voice sync failed; the current list is still usable",
      "gender.female": "Female",
      "gender.male": "Male",
      "error.textRequired": "Enter text to synthesize.",
      "error.engineUnavailable": "The selected engine is unavailable.",
      "error.voiceSyncing": "Voice list is syncing. Try again shortly.",
      "error.emptyAudio": "The service returned empty audio.",
      "error.synthesis": "Synthesis failed (HTTP {status})",
      "error.generic": "Synthesis failed. Try again.",
      "error.playback": "This audio cannot be played in the browser. Try downloading it.",
      "captcha.kicker": "SECURITY CHECK",
      "captcha.title": "Security check",
      "captcha.instruction": "Click these three icons in order:",
      "captcha.remaining": "Click {count} more icons",
      "captcha.verifying": "Verifying…",
      "captcha.success": "Verified",
      "captcha.failed": "Verification failed. Try clicking again.",
      "captcha.clear": "Clear",
      "captcha.undo": "Undo",
      "captcha.refresh": "Refresh",
      "captcha.close": "Close verification",
      "captcha.imageAlt": "Security image containing three geometric icons",
      "captcha.targetAlt": "Target icon {index}",
      "captcha.loadFailed": "Could not load the verification image. Try again.",
      "captcha.expired": "Verification expired. Click the icons again."
    }
  };

  let language = "zh";

  function interpolate(template, variables) {
    return String(template).replace(/\{(\w+)\}/g, (_, key) => {
      return variables && Object.hasOwn(variables, key) ? String(variables[key]) : `{${key}}`;
    });
  }

  function t(key, variables) {
    const dictionary = translations[language] || translations.zh;
    const fallback = translations.zh[key] || key;
    return interpolate(dictionary[key] || fallback, variables);
  }

  function applyLanguage(nextLanguage) {
    language = Object.hasOwn(translations, nextLanguage) ? nextLanguage : "zh";
    document.documentElement.lang = language === "zh" ? "zh-CN" : "en";
    document.title = t("page.title");
    localStorage.setItem("voxlab.uiLanguage", language);
    document.querySelectorAll("[data-i18n]").forEach((element) => {
      element.textContent = t(element.dataset.i18n);
    });
    document.querySelectorAll("[data-i18n-placeholder]").forEach((element) => {
      element.placeholder = t(element.dataset.i18nPlaceholder);
    });
    document.querySelectorAll("[data-i18n-alt]").forEach((element) => {
      element.alt = t(element.dataset.i18nAlt);
    });
    document.querySelectorAll("[data-i18n-aria]").forEach((element) => {
      element.setAttribute("aria-label", t(element.dataset.i18nAria));
    });
    document.querySelectorAll(".ui-language-select").forEach((select) => {
      select.value = language;
    });
    window.dispatchEvent(new CustomEvent("voxlab:languagechange", {
      detail: { language }
    }));
  }

  function current() {
    return language;
  }

  window.VoxLabI18n = { applyLanguage, current, t, translations };
})();
