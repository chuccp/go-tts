(() => {
  "use strict";

  const form = document.getElementById("tts-form");
  const text = document.getElementById("text");
  const voiceSelect = document.getElementById("voice");
  const voiceStatus = document.getElementById("voice-status");
  const voiceLocale = document.getElementById("voice-locale");
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
  let audioURL = "";
  let loadingVoices = false;
  let statusTimer = 0;

  const samples = {
    zh: "欢迎使用 VoxLab。这里汇聚了 Kokoro、Kitten 与 Edge TTS 三套语音引擎。输入一段文字，选择喜欢的声音，让文字自然地流动起来。",
    en: "Welcome to VoxLab. Three speech engines, one simple workspace. Choose a voice, set your pace, and bring every sentence to life.",
    quote: "声音不只是信息的载体，也是情绪的形状。"
  };

  function activeEngine() {
    return engines.find((engine) => engine.id === selectedEngine);
  }

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
    if (!option) {
      voiceLocale.textContent = "";
      return;
    }
    const bits = [option.dataset.language, option.dataset.locale, option.dataset.gender].filter(Boolean);
    voiceLocale.textContent = bits.join(" · ");
  }

  function populateVoices(voices, requestedVoice = "") {
    voiceSelect.replaceChildren();
    if (!voices || voices.length === 0) {
      const option = document.createElement("option");
      option.textContent = "没有可用音色";
      option.disabled = true;
      voiceSelect.append(option);
      updateVoiceMeta();
      return;
    }

    const groups = new Map();
    for (const voice of voices) {
      const key = [voice.language || voice.locale || "其他", voice.locale || ""].filter(Boolean).join(" · ");
      if (!groups.has(key)) {
        const group = document.createElement("optgroup");
        group.label = key;
        groups.set(key, group);
      }
      const option = document.createElement("option");
      option.value = voice.id;
      option.textContent = [voice.name, voice.gender].filter(Boolean).join(" · ");
      option.dataset.locale = voice.locale || "";
      option.dataset.gender = voice.gender || "";
      option.dataset.language = voice.language || "";
      groups.get(key).append(option);
    }
    voiceSelect.append(...groups.values());

    const preferred = voices.find((voice) => voice.id === requestedVoice);
    if (preferred) {
      voiceSelect.value = preferred.id;
    } else {
      voiceSelect.selectedIndex = 0;
    }
    updateVoiceMeta();
  }

  async function loadVoices(engineID) {
    const engine = engines.find((item) => item.id === engineID);
    if (!engine) {
      return;
    }
    populateVoices(engine.voices || [], selectedVoice);
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
        populateVoices(payload.voices, selectedVoice);
      }
      voiceStatus.textContent = payload.warning
        ? `在线音色暂不可用，已使用内置列表：${payload.warning}`
        : `${voiceSelect.options.length} 个音色`;
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
    clearError();
    document.querySelectorAll(".engine-card").forEach((card) => {
      const active = card.dataset.engine === engineID;
      card.setAttribute("aria-checked", String(active));
    });
    loadVoices(engineID);
  }

  document.querySelectorAll(".engine-card").forEach((card) => {
    card.addEventListener("click", () => selectEngine(card.dataset.engine));
  });

  document.querySelectorAll(".sample-chip").forEach((button) => {
    button.addEventListener("click", () => {
      text.value = samples[button.dataset.sample] || "";
      updateCharCount();
      text.focus();
    });
  });

  voiceSelect.addEventListener("change", updateVoiceMeta);
  speed.addEventListener("input", () => {
    speedValue.textContent = `${Number(speed.value).toFixed(2)}×`;
  });
  text.addEventListener("input", updateCharCount);

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
  if (selectedEngine) {
    loadVoices(selectedEngine);
  }
})();
