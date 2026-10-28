(() => {
  "use strict";

  const form = document.getElementById("tts-form");
  const uiLanguageSelects = Array.from(document.querySelectorAll(".ui-language-select"));
  const i18n = window.VoxLabI18n;
  const preferredUiLanguage = localStorage.getItem("voxlab.uiLanguage") ||
    (navigator.language.toLowerCase().startsWith("zh") ? "zh" : "en");
  i18n.applyLanguage(preferredUiLanguage);
  const text = document.getElementById("text");
  const workspace = document.querySelector(".workspace");
  const resultPanel = document.querySelector(".result-panel");
  const settingsGrid = document.querySelector(".settings-grid");
  const speedField = document.querySelector(".speed-field");
  const mobileSpeedSlot = document.getElementById("mobile-speed-slot");
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
  const captchaDialog = document.getElementById("captcha-dialog");
  const captchaClose = document.getElementById("captcha-close");
  const captchaImageWrap = document.getElementById("captcha-image-wrap");
  const captchaImage = document.getElementById("captcha-image");
  const captchaMarkerCanvas = document.getElementById("captcha-marker-canvas");
  const captchaTargets = document.getElementById("captcha-targets");
  const captchaStatus = document.getElementById("captcha-status");
  const captchaClear = document.getElementById("captcha-clear");
  const captchaUndo = document.getElementById("captcha-undo");
  const captchaRefresh = document.getElementById("captcha-refresh");

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
  let hasAudio = false;
  let isBusy = false;
  let lastOutput = null;
  let captchaChallenge = null;
  let captchaClicks = [];
  let captchaVerifying = false;
  let captchaSucceeded = false;
  let pendingCaptchaBody = null;

  const samples = {
    zh: "欢迎使用 VoxLab。这里汇聚了 Kokoro、Kitten 与 Edge TTS 三套语音引擎。输入一段文字，选择喜欢的声音，让文字自然地流动起来。",
    en: "Welcome to VoxLab. Three speech engines, one simple workspace. Choose a voice, set your pace, and bring every sentence to life.",
    quote: "声音不只是信息的载体，也是情绪的形状。"
  };

  function activeEngine() {
    return engines.find((engine) => engine.id === selectedEngine);
  }

  function tr(key, variables) {
    return i18n.t(key, variables);
  }

  function languageLabel(code, fallback) {
    if (!code) return fallback || "";
    const key = `language.${code}`;
    const translated = tr(key);
    return translated === key ? (fallback || code) : translated;
  }

  function refreshDynamicLanguage() {
    buttonLabel.textContent = isBusy ? tr("button.generating") : tr("button.generate");
    if (!hasAudio) {
      playerTitle.textContent = tr("player.waiting");
      playerDetail.textContent = tr("player.waitingHint");
    } else if (lastOutput) {
      playerTitle.textContent = lastOutput.title;
      playerDetail.textContent = tr("player.generatedDetail", {
        speed: lastOutput.speed,
        size: lastOutput.size
      });
    }
    if (detectedLanguage) {
      languageDetection.textContent = tr("detect.detected", {
        name: languageLabel(detectedLanguage.code, detectedLanguage.name),
        confidence: detectedLanguage.confidence
      });
    }
    if (!captchaDialog.hidden && !captchaVerifying && !captchaSucceeded) {
      updateCaptchaRemaining();
    }
    const engine = activeEngine();
    if (engine && languageSelect.options.length > 0) {
      populateLanguages(engine, languageSelect.value);
      populateVoices(engine, selectedVoice);
      if (!loadingVoices && voiceSelect.value) {
        voiceStatus.textContent = tr("voice.count", {
          count: voiceSelect.options.length,
          language: languageSelect.selectedOptions[0]?.dataset.name || ""
        });
      }
    }
    updateLanguageWarning();
  }

  const mobileLayout = window.matchMedia("(max-width: 640px)");
  function placeResultPanel() {
    if (mobileLayout.matches) {
      text.after(resultPanel);
      mobileSpeedSlot.append(speedField);
    } else {
      workspace.append(resultPanel);
      settingsGrid.append(speedField);
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
        languages.set(code, {
          code,
          name: languageLabel(code, voice.language || code),
          count: 0
        });
      }
      languages.get(code).count += 1;
    }

    const previous = preferredCode || languageSelect.value || selectedLanguage;
    languageSelect.replaceChildren();
    if (languages.size === 0) {
      const option = document.createElement("option");
      option.textContent = tr("role.empty");
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
      option.textContent = tr("role.empty");
      option.disabled = true;
      voiceSelect.append(option);
      updateVoiceMeta();
      updateLanguageWarning();
      return;
    }

    for (const voice of voices) {
      const option = document.createElement("option");
      option.value = voice.id;
      const genderKey = `gender.${String(voice.gender || "").toLowerCase()}`;
      const gender = voice.gender ? tr(genderKey) : "";
      option.textContent = [voice.name, gender].filter(Boolean).join(" · ");
      option.dataset.locale = voice.locale || "";
      option.dataset.gender = voice.gender || "";
      option.dataset.language = languageLabel(voiceLanguageCode(voice), voice.language || "");
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
    voiceStatus.textContent = tr("voice.syncing");
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
        ? tr("voice.fallback", { warning: payload.warning })
        : tr("voice.count", {
          count: voiceSelect.options.length,
          language: languageSelect.selectedOptions[0]?.dataset.name || ""
        });
    } catch (error) {
      voiceStatus.textContent = tr("voice.failed");
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
      languageWarning.textContent = tr("warning.unsupported", {
        language: languageLabel(detectedLanguage.code, detectedLanguage.name),
        engine: engine.name
      });
      languageWarning.hidden = false;
      return;
    }
    const currentCode = languageSelect.value;
    if (currentCode && currentCode !== detectedLanguage.code) {
      const currentName = languageSelect.selectedOptions[0]?.dataset.name || currentCode;
      languageWarning.textContent = tr("warning.mismatch", {
        detected: languageLabel(detectedLanguage.code, detectedLanguage.name),
        selected: languageLabel(currentCode, currentName)
      });
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
        languageDetection.textContent = tr("detect.unknown");
        updateLanguageWarning();
        return;
      }
      detectedLanguage = result;
      languageDetection.textContent = tr("detect.detected", {
        name: languageLabel(result.code, result.name),
        confidence: result.confidence
      });
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
        languageDetection.textContent = tr("detect.failed");
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
    languageDetection.textContent = tr("detect.detecting");
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
      showError(tr("error.playback"));
    }
  });

  function setBusy(busy) {
    synthesize.disabled = busy;
    synthesize.classList.toggle("is-loading", busy);
    isBusy = busy;
    buttonLabel.textContent = busy ? tr("button.generating") : tr("button.generate");
    if (busy) {
      const started = Date.now();
      statusTimer = window.setInterval(() => {
        buttonLabel.textContent = tr("button.generatingSeconds", {
          seconds: Math.floor((Date.now() - started) / 1000)
        });
      }, 1000);
    } else {
      window.clearInterval(statusTimer);
      statusTimer = 0;
    }
  }

  function setCaptchaStatus(message, state = "") {
    captchaStatus.textContent = message;
    captchaStatus.classList.toggle("is-success", state === "success");
    captchaStatus.classList.toggle("is-error", state === "error");
  }

  function updateCaptchaRemaining() {
    const remaining = Math.max(0, 3 - captchaClicks.length);
    setCaptchaStatus(tr("captcha.remaining", { count: remaining }));
  }

  function drawCaptchaMarkers() {
    const width = captchaImage.naturalWidth || 300;
    const height = captchaImage.naturalHeight || 180;
    if (captchaMarkerCanvas.width !== width || captchaMarkerCanvas.height !== height) {
      captchaMarkerCanvas.width = width;
      captchaMarkerCanvas.height = height;
    }
    const context = captchaMarkerCanvas.getContext("2d");
    if (!context) {
      return;
    }
    context.clearRect(0, 0, width, height);
    captchaClicks.forEach((click, index) => {
      context.beginPath();
      context.arc(click.x, click.y, 12, 0, Math.PI * 2);
      context.fillStyle = "#d8ff67";
      context.fill();
      context.lineWidth = 2;
      context.strokeStyle = "#f4f5ee";
      context.stroke();
      context.fillStyle = "#182008";
      context.font = "bold 12px sans-serif";
      context.textAlign = "center";
      context.textBaseline = "middle";
      context.fillText(String(index + 1), click.x, click.y + 0.5);
    });
  }

  function resetCaptchaClicks() {
    captchaClicks = [];
    captchaVerifying = false;
    captchaSucceeded = false;
    drawCaptchaMarkers();
    updateCaptchaRemaining();
  }

  function applyCaptchaChallenge(challenge) {
    if (!challenge?.image || !Array.isArray(challenge.targets) ||
      challenge.targets.length !== 3 || !challenge?.clickCode) {
      throw new Error(tr("captcha.loadFailed"));
    }
    captchaChallenge = challenge;
    captchaTargets.replaceChildren();
    challenge.targets.forEach((target, index) => {
      const image = document.createElement("img");
      image.src = target;
      image.alt = tr("captcha.targetAlt", { index: index + 1 });
      image.draggable = false;
      captchaTargets.append(image);
    });
    captchaImage.src = challenge.image;
    resetCaptchaClicks();
  }

  function openCaptcha(challenge, body) {
    pendingCaptchaBody = body;
    applyCaptchaChallenge(challenge);
    captchaDialog.hidden = false;
    document.body.classList.add("captcha-open");
    window.setTimeout(() => captchaClose.focus(), 0);
  }

  function closeCaptcha() {
    captchaDialog.hidden = true;
    document.body.classList.remove("captcha-open");
    pendingCaptchaBody = null;
    captchaChallenge = null;
    captchaClicks = [];
    captchaVerifying = false;
    captchaSucceeded = false;
  }

  function handleCaptchaImageClick(event) {
    if (!captchaChallenge || captchaVerifying || captchaSucceeded || captchaClicks.length >= 3) {
      return;
    }
    const rect = captchaImage.getBoundingClientRect();
    if (rect.width <= 0 || rect.height <= 0) {
      return;
    }
    const width = captchaImage.naturalWidth || 300;
    const height = captchaImage.naturalHeight || 180;
    captchaClicks.push({
      x: (event.clientX - rect.left) * (width / rect.width),
      y: (event.clientY - rect.top) * (height / rect.height)
    });
    drawCaptchaMarkers();
    if (captchaClicks.length >= 3) {
      verifyCaptchaClicks();
    } else {
      updateCaptchaRemaining();
    }
  }

  async function verifyCaptchaClicks() {
    if (!captchaChallenge || !pendingCaptchaBody || captchaVerifying) {
      return;
    }
    captchaVerifying = true;
    setCaptchaStatus(tr("captcha.verifying"));
    const body = new URLSearchParams({
      clickCode: captchaChallenge.clickCode,
      clickXs: captchaClicks.map((click) => Math.round(click.x)).join(","),
      clickYs: captchaClicks.map((click) => Math.round(click.y)).join(",")
    });
    try {
      const response = await fetch("/api/captcha/verify", {
        method: "POST",
        headers: {
          "Content-Type": "application/x-www-form-urlencoded;charset=UTF-8",
          Accept: "application/json"
        },
        body
      });
      const payload = await response.json().catch(() => ({}));
      if (!response.ok || !payload.ticket) {
        captchaVerifying = false;
        if (payload.captcha) {
          applyCaptchaChallenge(payload.captcha);
        } else {
          captchaClicks = [];
          drawCaptchaMarkers();
        }
        setCaptchaStatus(tr("captcha.failed"), "error");
        return;
      }
      captchaSucceeded = true;
      setCaptchaStatus(tr("captcha.success"), "success");
      const synthesisBody = pendingCaptchaBody;
      captchaDialog.hidden = true;
      document.body.classList.remove("captcha-open");
      pendingCaptchaBody = null;
      window.setTimeout(() => requestSynthesis(synthesisBody, payload.ticket), 350);
    } catch (_) {
      captchaVerifying = false;
      captchaClicks = [];
      drawCaptchaMarkers();
      setCaptchaStatus(tr("captcha.loadFailed"), "error");
    }
  }

  async function refreshCaptcha() {
    if (!pendingCaptchaBody || captchaVerifying) {
      return;
    }
    try {
      const response = await fetch("/api/captcha/challenge", {
        headers: { Accept: "application/json" }
      });
      const payload = await response.json();
      if (!response.ok || !payload.captcha) {
        throw new Error(tr("captcha.loadFailed"));
      }
      applyCaptchaChallenge(payload.captcha);
    } catch (_) {
      setCaptchaStatus(tr("captcha.loadFailed"), "error");
    }
  }

  async function requestSynthesis(body, ticket = "") {
    const engine = activeEngine();
    const requestBody = new URLSearchParams(body);
    if (ticket) {
      requestBody.set("captchaTicket", ticket);
    }
    setBusy(true);
    try {
      const response = await fetch("/api/synthesize", {
        method: "POST",
        headers: {
          "Content-Type": "application/x-www-form-urlencoded;charset=UTF-8",
          Accept: "audio/*, application/json"
        },
        body: requestBody
      });
      if (response.status === 428) {
        const payload = await response.json().catch(() => ({}));
        if (payload.captchaRequired && payload.captcha) {
          setBusy(false);
          openCaptcha(payload.captcha, body);
          return;
        }
      }
      if (!response.ok) {
        let message = tr("error.synthesis", { status: response.status });
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
        throw new Error(tr("error.emptyAudio"));
      }
      if (audioURL) {
        URL.revokeObjectURL(audioURL);
      }
      audioURL = URL.createObjectURL(blob);
      audio.src = audioURL;
      audio.load();
      audio.play().catch(() => {});

      const option = voiceSelect.selectedOptions[0];
      const outputTitle = `${engine.name} · ${option?.textContent || voiceSelect.value}`;
      lastOutput = {
        title: outputTitle,
        speed: Number(speed.value).toFixed(2),
        size: (blob.size / 1024).toFixed(0)
      };
      hasAudio = true;
      playerTitle.textContent = outputTitle;
      playerDetail.textContent = tr("player.generatedDetail", {
        speed: lastOutput.speed,
        size: lastOutput.size
      });
      audio.removeAttribute("aria-disabled");
      download.href = audioURL;
      download.download = `voxlab-${selectedEngine}-${Date.now()}.${blob.type.includes("wav") ? "wav" : "mp3"}`;
      download.classList.remove("is-disabled");
      download.removeAttribute("aria-disabled");
      resultPlaceholder.hidden = true;
      player.scrollIntoView({ behavior: "smooth", block: "nearest" });
    } catch (error) {
      showError(error instanceof Error ? error.message : tr("error.generic"));
    } finally {
      setBusy(false);
    }
  }

  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    clearError();
    const content = text.value.trim();
    if (!content) {
      showError(tr("error.textRequired"));
      text.focus();
      return;
    }
    const engine = activeEngine();
    if (!engine?.available) {
      showError(engine?.error || tr("error.engineUnavailable"));
      return;
    }
    if (loadingVoices) {
      showError(tr("error.voiceSyncing"));
      return;
    }

    const body = new URLSearchParams({
      engine: selectedEngine,
      language: languageSelect.value,
      voice: voiceSelect.value,
      text: content,
      speed: speed.value
    });
    await requestSynthesis(body);
  });

  captchaImage.addEventListener("load", drawCaptchaMarkers);
  captchaImage.addEventListener("error", () => {
    if (!captchaDialog.hidden) {
      setCaptchaStatus(tr("captcha.loadFailed"), "error");
    }
  });
  captchaImageWrap.addEventListener("click", handleCaptchaImageClick);
  captchaClear.addEventListener("click", () => {
    if (!captchaVerifying && !captchaSucceeded) {
      resetCaptchaClicks();
    }
  });
  captchaUndo.addEventListener("click", () => {
    if (!captchaVerifying && !captchaSucceeded && captchaClicks.length > 0) {
      captchaClicks.pop();
      drawCaptchaMarkers();
      updateCaptchaRemaining();
    }
  });
  captchaRefresh.addEventListener("click", refreshCaptcha);
  document.querySelectorAll("[data-captcha-close]").forEach((element) => {
    element.addEventListener("click", closeCaptcha);
  });
  document.addEventListener("keydown", (event) => {
    if (event.key === "Escape" && !captchaDialog.hidden) {
      closeCaptcha();
    }
  });

  uiLanguageSelects.forEach((select) => {
    select.addEventListener("change", () => {
      i18n.applyLanguage(select.value);
    });
  });
  window.addEventListener("voxlab:languagechange", refreshDynamicLanguage);

  updateCharCount();
  refreshDynamicLanguage();
  if (selectedEngine) {
    loadVoices(selectedEngine);
  }
})();
