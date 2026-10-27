(() => {
  "use strict";

  const form = document.getElementById("tts-form");
  const text = document.getElementById("text");
  const workspace = document.querySelector(".workspace");
  const resultPanel = document.querySelector(".result-panel");
  const languageSelect = document.getElementById("language");
  const voiceSelect = document.getElementById("voice");
  const voiceStatus = document.getElementById("voice-status");
  const voiceLocale = document.getElementById("voice-locale");
  const languageDetection = document.getElementById("language-detection");
  const languageWarning = document.getElementById("language-warning");
  const speed = document.getElementById("speed");
  const speedValue = document.getElementById("speed-value");
  const charCount = document.getElementById("char-count");
  const synthesize = document.getElementById("synthesize");
  const buttonLabel = synthesize.querySelector(".button-label");
  const errorBox = document.getElementById("error-box");
  const player = document.getElementById("player");
  const audio = document.getElementById("audio");
  const playerTitle = document.getElementById("player-title");
  const playerDetail = document.getElementById("player-detail");
  const audioDuration = document.getElementById("audio-duration");
  const download = document.getElementById("download");
  const resultPlaceholder = document.querySelector(".result-placeholder");

  const engines = JSON.parse(document.getElementById("engine-data").textContent);
  const initialEngine = document.querySelector(".engine-card[aria-checked='true']");
  let selectedEngine = initialEngine?.dataset.engine || engines[0]?.id || "";
  let selectedVoice = voiceSelect.value;
  let selectedLanguage = languageSelect.value;
  let detectedLanguage = null;
  let audioURL = "";
  let loadingVoices = false;
  let statusTimer = 0;
  let detectionTimer = 0;
  let detectionSequence = 0;

  const samples = {
    zh: "欢迎使用 VoxLab。这里汇聚了 Kokoro、Kitten 与 Edge TTS 三套语音引擎。输入一段文字，选择喜欢的声音，让文字自然地流动起来。",
    en: "Welcome to VoxLab. Three speech engines, one simple workspace. Choose a voice, set your pace, and bring every sentence to life.",
    quote: "声音不只是信息的载体，也是情绪的形状。"
  };

  function activeEngine() {
    return engines.find((engine) => engine.id === selectedEngine);
  }

  const mobileLayout = window.matchMedia("(max-width: 640px)");
  function placeResultPanel() {
    if (mobileLayout.matches) {
      text.after(resultPanel);
    } else {
      workspace.append(resultPanel);
    }
  }
  placeResultPanel();
  mobileLayout.addEventListener("change", placeResultPanel);

  function showError(message) {
    errorBox.textContent = message;
    errorBox.hidden = false;
  }

  function clearError() {
    errorBox.textContent = "";
    errorBox.hidden = true;
  }

  function formatDuration(seconds) {
    if (!Number.isFinite(seconds) || seconds < 0) {
      return "00:00";
    }
    const minutes = Math.floor(seconds / 60);
    const rest = Math.floor(seconds % 60);
    return `${String(minutes).padStart(2, "0")}:${String(rest).padStart(2, "0")}`;
  }

  function updateCharCount() {
    const count = Array.from(text.value).length;
    charCount.textContent = `${count} / ${charCount.dataset.max || charCount.textContent.split("/").pop().trim()}`;
  }

  charCount.dataset.max = charCount.textContent.split("/").pop().trim();

  function updateVoiceMeta() {
    const option = voiceSelect.selectedOptions[0];
    selectedVoice = voiceSelect.value;
    selectedLanguage = languageSelect.value;
    if (!option) {
      voiceLocale.textContent = "";
      return;
    }
    const bits = [option.dataset.language, option.dataset.locale, option.dataset.gender].filter(Boolean);
    voiceLocale.textContent = bits.join(" · ");
  }

  function voiceLanguageCode(voice) {
    return voice.languageCode || String(voice.locale || "").split("-")[0].toLowerCase();
  }

  function activeVoices(engine) {
    return Array.isArray(engine?.voices) ? engine.voices : [];
  }

  function populateLanguages(engine, preferredCode = "") {
    const voices = activeVoices(engine);
    const languages = new Map();
    for (const voice of voices) {
      const code = voiceLanguageCode(voice);
      if (!code) {
        continue;
      }
      if (!languages.has(code)) {
        languages.set(code, { code, name: voice.language || code, count: 0 });
      }
      languages.get(code).count += 1;
    }

    const previous = preferredCode || languageSelect.value || selectedLanguage;
    languageSelect.replaceChildren();
    if (languages.size === 0) {
      const option = document.createElement("option");
      option.textContent = "无可用语言";
      option.disabled = true;
      languageSelect.append(option);
      selectedLanguage = "";
      return;
    }

    const items = Array.from(languages.values());
    const defaultVoice = voices.find((voice) => voice.id === engine?.defaultVoice);
    const priority = new Map([["zh", 0], ["en", 1], ["ja", 2], ["ko", 3]]);
    items.sort((a, b) => {
      if (a.code === previous) return -1;
      if (b.code === previous) return 1;
      const ap = priority.has(a.code) ? priority.get(a.code) : 99;
      const bp = priority.has(b.code) ? priority.get(b.code) : 99;
      return ap - bp || a.name.localeCompare(b.name);
    });
    for (const language of items) {
      const option = document.createElement("option");
      option.value = language.code;
      option.textContent = `${language.name} · ${language.count}`;
      option.dataset.name = language.name;
      languageSelect.append(option);
    }

    let target = previous;
    if (!languages.has(target) && detectedLanguage && languages.has(detectedLanguage.code)) {
      target = detectedLanguage.code;
    }
    if (!languages.has(target) && defaultVoice && languages.has(voiceLanguageCode(defaultVoice))) {
      target = voiceLanguageCode(defaultVoice);
    }
    if (!languages.has(target)) {
      target = items[0].code;
    }
    languageSelect.value = target;
    selectedLanguage = target;
  }

  function populateVoices(engine, requestedVoice = "") {
    const allVoices = activeVoices(engine);
    const voices = allVoices.filter((voice) => voiceLanguageCode(voice) === languageSelect.value);
    voiceSelect.replaceChildren();
    if (voices.length === 0) {
      const option = document.createElement("option");
      option.textContent = "没有可用角色";
      option.disabled = true;
      voiceSelect.append(option);
      updateVoiceMeta();
      updateLanguageWarning();
      return;
    }

    for (const voice of voices) {
      const option = document.createElement("option");
      option.value = voice.id;
      option.textContent = [voice.name, voice.gender].filter(Boolean).join(" · ");
      option.dataset.locale = voice.locale || "";
      option.dataset.gender = voice.gender || "";
      option.dataset.language = voice.language || "";
      option.dataset.languageCode = voiceLanguageCode(voice);
      voiceSelect.append(option);
    }

    const preferred = voices.find((voice) => voice.id === requestedVoice);
    if (preferred) {
      voiceSelect.value = preferred.id;
    } else {
      voiceSelect.selectedIndex = 0;
    }
    updateVoiceMeta();
    updateLanguageWarning();
  }

  async function loadVoices(engineID) {
    const engine = engines.find((item) => item.id === engineID);
    if (!engine) {
      return;
    }
    const preferredLanguage = detectedLanguage?.code || selectedLanguage;
    populateLanguages(engine, preferredLanguage);
    populateVoices(engine, selectedVoice);
    if (!engine.available) {
      voiceStatus.textContent = engine.error || "引擎未就绪";
      return;
    }

    loadingVoices = true;
    voiceStatus.textContent = "正在同步音色列表…";
    try {
      const response = await fetch(`/api/voices?engine=${encodeURIComponent(engineID)}`, {
        headers: { Accept: "application/json" }
      });
      const payload = await response.json();
      if (!response.ok) {
        throw new Error(payload.error || `HTTP ${response.status}`);
      }
      if (Array.isArray(payload.voices) && payload.voices.length > 0) {
        engine.voices = payload.voices;
        populateLanguages(engine, detectedLanguage?.code || selectedLanguage);
        populateVoices(engine, selectedVoice);
      }
      voiceStatus.textContent = payload.warning
        ? `在线音色暂不可用，已使用内置列表：${payload.warning}`
        : `${voiceSelect.options.length} 个${languageSelect.selectedOptions[0]?.dataset.name || ""}角色`;
    } catch (error) {
      voiceStatus.textContent = "音色同步失败，可继续使用当前列表";
    } finally {
      loadingVoices = false;
    }
  }

  function selectEngine(engineID) {
    const engine = engines.find((item) => item.id === engineID);
    if (!engine || !engine.available) {
      return;
    }
    selectedEngine = engineID;
    selectedVoice = engine.defaultVoice || "";
    selectedLanguage = "";
    clearError();
    document.querySelectorAll(".engine-card").forEach((card) => {
      const active = card.dataset.engine === engineID;
      card.setAttribute("aria-checked", String(active));
    });
    loadVoices(engineID);
  }

  function updateLanguageWarning() {
    const engine = activeEngine();
    if (!engine || !detectedLanguage?.code) {
      languageWarning.hidden = true;
      return;
    }
    const voices = activeVoices(engine);
    const supported = voices.some((voice) => voiceLanguageCode(voice) === detectedLanguage.code);
    if (!supported) {
      languageWarning.textContent = `检测到${detectedLanguage.name}，${engine.name} 当前没有该语言的角色。`;
      languageWarning.hidden = false;
      return;
    }
    const currentCode = languageSelect.value;
    if (currentCode && currentCode !== detectedLanguage.code) {
      const currentName = languageSelect.selectedOptions[0]?.dataset.name || currentCode;
      languageWarning.textContent = `检测到${detectedLanguage.name}，当前角色语言为${currentName}，请确认选择。`;
      languageWarning.hidden = false;
      return;
    }
    languageWarning.hidden = true;
  }

  async function detectInputLanguage(content) {
    const sequence = ++detectionSequence;
    try {
      const body = new URLSearchParams({ text: content });
      const response = await fetch("/api/detect-language", {
        method: "POST",
        headers: { "Content-Type": "application/x-www-form-urlencoded;charset=UTF-8" },
        body
      });
      const result = await response.json();
      if (sequence !== detectionSequence) {
        return;
      }
      if (!response.ok || !result.code) {
        detectedLanguage = null;
        languageDetection.textContent = "语言：未识别";
        updateLanguageWarning();
        return;
      }
      detectedLanguage = result;
      languageDetection.textContent = `检测到：${result.name} · ${result.confidence}%`;
      const engine = activeEngine();
      const supported = activeVoices(engine).some((voice) => voiceLanguageCode(voice) === result.code);
      if (supported && languageSelect.value !== result.code) {
        populateLanguages(engine, result.code);
        populateVoices(engine, "");
      } else {
        updateLanguageWarning();
      }
    } catch (_) {
      if (sequence === detectionSequence) {
        detectedLanguage = null;
        languageDetection.textContent = "语言检测失败";
        updateLanguageWarning();
      }
    }
  }

  function scheduleLanguageDetection() {
    window.clearTimeout(detectionTimer);
    const content = text.value.trim();
    if (!content) {
      detectionSequence++;
      detectedLanguage = null;
      languageDetection.textContent = "";
      updateLanguageWarning();
      return;
    }
    languageDetection.textContent = "语言检测中…";
    detectionTimer = window.setTimeout(() => detectInputLanguage(content), 350);
  }

  document.querySelectorAll(".engine-card").forEach((card) => {
    card.addEventListener("click", () => selectEngine(card.dataset.engine));
  });

  document.querySelectorAll(".sample-chip").forEach((button) => {
    button.addEventListener("click", () => {
      text.value = samples[button.dataset.sample] || "";
      updateCharCount();
      scheduleLanguageDetection();
      text.focus();
    });
  });

  languageSelect.addEventListener("change", () => {
    populateVoices(activeEngine(), "");
  });
  voiceSelect.addEventListener("change", () => {
    updateVoiceMeta();
    updateLanguageWarning();
  });
  speed.addEventListener("input", () => {
    speedValue.textContent = `${Number(speed.value).toFixed(2)}×`;
  });
  text.addEventListener("input", () => {
    updateCharCount();
    scheduleLanguageDetection();
  });

  audio.addEventListener("loadedmetadata", () => {
    audioDuration.textContent = formatDuration(audio.duration);
  });
  audio.addEventListener("error", () => {
    if (audio.src) {
      showError("浏览器无法播放这段音频，请尝试下载后播放。");
    }
  });

  function setBusy(busy) {
    synthesize.disabled = busy;
    synthesize.classList.toggle("is-loading", busy);
    buttonLabel.textContent = busy ? "正在生成" : "生成语音";
    if (busy) {
      const started = Date.now();
      statusTimer = window.setInterval(() => {
        buttonLabel.textContent = `正在生成 ${Math.floor((Date.now() - started) / 1000)}s`;
      }, 1000);
    } else {
      window.clearInterval(statusTimer);
      statusTimer = 0;
    }
  }

  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    clearError();
    const content = text.value.trim();
    if (!content) {
      showError("请先输入要转换的文字。");
      text.focus();
      return;
    }
    const engine = activeEngine();
    if (!engine?.available) {
      showError(engine?.error || "所选引擎当前不可用。");
      return;
    }
    if (loadingVoices) {
      showError("音色列表正在同步，请稍后再试。");
      return;
    }

    const body = new URLSearchParams({
      engine: selectedEngine,
      language: languageSelect.value,
      voice: voiceSelect.value,
      text: content,
      speed: speed.value
    });
    setBusy(true);
    try {
      const response = await fetch("/api/synthesize", {
        method: "POST",
        headers: {
          "Content-Type": "application/x-www-form-urlencoded;charset=UTF-8",
          Accept: "audio/*, application/json"
        },
        body
      });
      if (!response.ok) {
        let message = `合成失败（HTTP ${response.status}）`;
        try {
          const payload = await response.json();
          message = payload.error || message;
        } catch (_) {
          // Keep the status-based message for non-JSON errors.
        }
        throw new Error(message);
      }
      const blob = await response.blob();
      if (blob.size === 0) {
        throw new Error("服务返回了空音频。");
      }
      if (audioURL) {
        URL.revokeObjectURL(audioURL);
      }
      audioURL = URL.createObjectURL(blob);
      audio.src = audioURL;
      audio.load();
      audio.play().catch(() => {});

      const option = voiceSelect.selectedOptions[0];
      playerTitle.textContent = `${engine.name} · ${option?.textContent || voiceSelect.value}`;
      playerDetail.textContent = `${Number(speed.value).toFixed(2)}× 语速 · ${(blob.size / 1024).toFixed(0)} KB`;
      audio.removeAttribute("aria-disabled");
      download.href = audioURL;
      download.download = `voxlab-${selectedEngine}-${Date.now()}.${blob.type.includes("wav") ? "wav" : "mp3"}`;
      download.classList.remove("is-disabled");
      download.removeAttribute("aria-disabled");
      resultPlaceholder.hidden = true;
      player.scrollIntoView({ behavior: "smooth", block: "nearest" });
    } catch (error) {
      showError(error instanceof Error ? error.message : "合成失败，请稍后重试。");
    } finally {
      setBusy(false);
    }
  });

  updateCharCount();
  updateVoiceMeta();
  updateLanguageWarning();
  if (selectedEngine) {
    loadVoices(selectedEngine);
  }
})();
