import "./style.css";

const DEFAULT_SERVER = "https://zz.31pk.top";
const SERVER_KEY = "zfile_server_url";
const HISTORY_KEY = "zfile_upload_history";
const EXPIRE_MS = 24 * 60 * 60 * 1000;
const MAX_UPLOAD_BYTES = 500 * 1024 * 1024;
const MAX_UPLOAD_LABEL = "500MB";

let selectedFiles = [];
let lastCode = "";
let unlistenDownloadProgress = null;
let tauriApiPromise = null;

function loadTauriApi() {
  if (!tauriApiPromise) {
    tauriApiPromise = Promise.all([
      import("@tauri-apps/api/tauri"),
      import("@tauri-apps/api/event"),
    ]).then(([tauri, event]) => ({
      invoke: tauri.invoke,
      listen: event.listen,
    }));
  }
  return tauriApiPromise;
}

const $ = (id) => document.getElementById(id);

const sendTab = $("sendTab");
const recvTab = $("recvTab");
const sendPage = $("sendPage");
const recvPage = $("recvPage");
const dropZone = $("dropZone");
const fileInput = $("fileInput");
const fileName = $("fileName");
const uploadBtn = $("uploadBtn");
const uploadProgress = $("uploadProgress");
const uploadBar = $("uploadBar");
const resultCard = $("resultCard");
const pickupCode = $("pickupCode");
const copyBtn = $("copyBtn");
const codeInput = $("codeInput");
const downloadBtn = $("downloadBtn");
const downloadProgress = $("downloadProgress");
const downloadBar = $("downloadBar");
const statusTitle = $("statusTitle");
const statusSub = $("statusSub");
const autoOpen = $("autoOpen");
const autoUnzip = $("autoUnzip");
const autoStart = $("autoStart");
const settingsBtn = $("settingsBtn");
const settingsModal = $("settingsModal");
const serverInput = $("serverInput");
const serverTestStatus = $("serverTestStatus");
const cancelSettingsBtn = $("cancelSettingsBtn");
const testServerBtn = $("testServerBtn");
const saveServerBtn = $("saveServerBtn");
const historyList = $("historyList");
const historyEmpty = $("historyEmpty");
const clearHistoryBtn = $("clearHistoryBtn");

let testedServerUrl = "";

function normalizeServerUrl(value) {
  let url = (value || "").trim();
  if (!url) return DEFAULT_SERVER;
  url = url.replace(/\/+$/, "");
  if (!/^https?:\/\//i.test(url)) {
    url = `http://${url}`;
  }
  return url;
}

function getServer() {
  return normalizeServerUrl(localStorage.getItem(SERVER_KEY) || DEFAULT_SERVER);
}

function setServer(value) {
  const url = normalizeServerUrl(value);
  localStorage.setItem(SERVER_KEY, url);
  return url;
}

function formatSize(bytes) {
  if (!bytes) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let size = bytes;
  let unit = 0;
  while (size >= 1024 && unit < units.length - 1) {
    size /= 1024;
    unit += 1;
  }
  return `${unit === 0 ? size : size.toFixed(1)} ${units[unit]}`;
}

function formatExpireTime(timestamp) {
  const date = new Date(timestamp);
  const month = String(date.getMonth() + 1).padStart(2, "0");
  const day = String(date.getDate()).padStart(2, "0");
  const hour = String(date.getHours()).padStart(2, "0");
  const minute = String(date.getMinutes()).padStart(2, "0");
  return `${month}-${day} ${hour}:${minute}`;
}

function loadHistory() {
  try {
    const now = Date.now();
    const records = JSON.parse(localStorage.getItem(HISTORY_KEY) || "[]").filter(
      (item) => item?.code && item?.expiresAt && item.expiresAt > now,
    );
    localStorage.setItem(HISTORY_KEY, JSON.stringify(records));
    return records;
  } catch (_) {
    localStorage.removeItem(HISTORY_KEY);
    return [];
  }
}

function saveHistory(records) {
  localStorage.setItem(HISTORY_KEY, JSON.stringify(records.slice(0, 30)));
}

function summarizeFiles(files) {
  const list = Array.from(files);
  return {
    name: list.length === 1 ? list[0].name : `${list.length} 个文件`,
    count: list.length,
    size: list.reduce((sum, file) => sum + (file.size || 0), 0),
  };
}

function getTotalFileSize(files) {
  return Array.from(files).reduce((sum, file) => sum + (file.size || 0), 0);
}

function isUploadTooLarge(files) {
  return getTotalFileSize(files) > MAX_UPLOAD_BYTES;
}

function addHistoryRecord(code, files) {
  const summary = summarizeFiles(files);
  const now = Date.now();
  const record = {
    code,
    fileName: summary.name,
    fileCount: summary.count,
    fileSize: summary.size,
    server: getServer(),
    uploadedAt: now,
    expiresAt: now + EXPIRE_MS,
  };
  const records = loadHistory().filter((item) => !(item.code === code && item.server === record.server));
  records.unshift(record);
  saveHistory(records);
  renderHistory();
}

async function copyText(text) {
  try {
    await navigator.clipboard.writeText(text);
  } catch (_) {
    const textarea = document.createElement("textarea");
    textarea.value = text;
    textarea.style.position = "fixed";
    textarea.style.left = "-9999px";
    document.body.appendChild(textarea);
    textarea.select();
    document.execCommand("copy");
    document.body.removeChild(textarea);
  }
}

function renderHistory() {
  const records = loadHistory();
  historyList.innerHTML = "";
  historyEmpty.classList.toggle("hidden", records.length > 0);
  clearHistoryBtn.classList.toggle("hidden", records.length === 0);

  records.slice(0, 3).forEach((item) => {
    const row = document.createElement("div");
    row.className = "history-item";

    const info = document.createElement("div");
    const name = document.createElement("div");
    name.className = "history-name";
    name.textContent = item.fileName || "未知文件";
    const meta = document.createElement("div");
    meta.className = "history-meta";
    meta.textContent = `${item.code} · ${formatSize(item.fileSize)} · 过期 ${formatExpireTime(item.expiresAt)}`;
    info.append(name, meta);

    const actions = document.createElement("div");
    actions.className = "history-actions";
    const useBtn = document.createElement("button");
    useBtn.type = "button";
    useBtn.textContent = "填入";
    useBtn.addEventListener("click", () => {
      codeInput.value = item.code;
      switchPage("recv");
      codeInput.focus();
    });
    const copyHistoryBtn = document.createElement("button");
    copyHistoryBtn.type = "button";
    copyHistoryBtn.textContent = "复制";
    copyHistoryBtn.addEventListener("click", async () => {
      await copyText(item.code);
      copyHistoryBtn.textContent = "已复制";
      setTimeout(() => (copyHistoryBtn.textContent = "复制"), 1200);
    });
    actions.append(useBtn, copyHistoryBtn);
    row.append(info, actions);
    historyList.appendChild(row);
  });
}

function setServerStatus(message, type = "") {
  serverTestStatus.textContent = message;
  serverTestStatus.classList.toggle("ok", type === "ok");
  serverTestStatus.classList.toggle("error", type === "error");
}

function openSettings() {
  testedServerUrl = "";
  serverInput.value = getServer();
  saveServerBtn.disabled = true;
  setServerStatus(`当前生效：${getServer()}`);
  settingsModal.classList.remove("hidden");
  setTimeout(() => serverInput.focus(), 0);
}

function closeSettings() {
  settingsModal.classList.add("hidden");
}

async function testServerConnection() {
  const url = normalizeServerUrl(serverInput.value);
  testServerBtn.disabled = true;
  saveServerBtn.disabled = true;
  testedServerUrl = "";
  setServerStatus(`正在测试：${url}/health`);

  try {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), 8000);
    const response = await fetch(`${url}/health`, {
      method: "GET",
      cache: "no-store",
      signal: controller.signal,
    });
    clearTimeout(timer);
    const body = (await response.text()).trim().toLowerCase();
    if (!response.ok || body !== "ok") {
      throw new Error(`HTTP ${response.status}`);
    }
    testedServerUrl = url;
    saveServerBtn.disabled = false;
    setServerStatus(`连接成功，可以保存：${url}`, "ok");
  } catch (error) {
    setServerStatus(`连接失败，请检查地址、端口或反向代理：${error.message || error}`, "error");
  } finally {
    testServerBtn.disabled = false;
  }
}

function switchPage(page) {
  const isSend = page === "send";
  sendTab.classList.toggle("active", isSend);
  recvTab.classList.toggle("active", !isSend);
  sendPage.classList.toggle("active", isSend);
  recvPage.classList.toggle("active", !isSend);
}

function setFiles(files) {
  if (isUploadTooLarge(files)) {
    selectedFiles = [];
    fileInput.value = "";
    fileName.textContent = `文件超过上传限制，最大允许 ${MAX_UPLOAD_LABEL}`;
    fileName.classList.add("show");
    resultCard.classList.add("hidden");
    return;
  }
  selectedFiles = Array.from(files);
  if (!selectedFiles.length) return;
  fileName.textContent =
    selectedFiles.length === 1
      ? `📄 ${selectedFiles[0].name}`
      : `📄 已选择 ${selectedFiles.length} 个文件`;
  fileName.classList.add("show");
  resultCard.classList.add("hidden");
}

function setStatus(title, sub, error = false) {
  statusTitle.textContent = title;
  statusSub.textContent = sub;
  statusTitle.classList.toggle("error", error);
}

function resetUploadProgress() {
  uploadBar.style.width = "0%";
  uploadProgress.classList.add("hidden");
}

function resetDownloadProgress() {
  downloadBar.style.width = "0%";
  downloadProgress.classList.add("hidden");
}

function uploadWithProgress(files) {
  return new Promise((resolve, reject) => {
    const form = new FormData();
    files.forEach((file) => form.append("file", file));

    const xhr = new XMLHttpRequest();
    xhr.open("POST", `${getServer()}/upload`);
    xhr.timeout = 0;

    xhr.upload.onprogress = (event) => {
      if (!event.lengthComputable) return;
      const percent = Math.max(2, Math.min(99, Math.round((event.loaded / event.total) * 100)));
      uploadBar.style.width = `${percent}%`;
    };

    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        resolve(xhr.responseText.trim());
      } else {
        reject(new Error(xhr.responseText || `HTTP ${xhr.status}`));
      }
    };

    xhr.onerror = () => reject(new Error("网络连接失败"));
    xhr.ontimeout = () => reject(new Error("上传超时"));
    xhr.send(form);
  });
}

sendTab.addEventListener("click", () => switchPage("send"));
recvTab.addEventListener("click", () => switchPage("recv"));

dropZone.addEventListener("click", () => fileInput.click());
fileInput.addEventListener("change", () => setFiles(fileInput.files));

["dragenter", "dragover"].forEach((eventName) => {
  dropZone.addEventListener(eventName, (event) => {
    event.preventDefault();
    dropZone.classList.add("dragging");
  });
});

["dragleave", "drop"].forEach((eventName) => {
  dropZone.addEventListener(eventName, (event) => {
    event.preventDefault();
    dropZone.classList.remove("dragging");
  });
});

dropZone.addEventListener("drop", (event) => {
  const files = event.dataTransfer?.files;
  if (files?.length) setFiles(files);
});

uploadBtn.addEventListener("click", async () => {
  if (!selectedFiles.length) {
    fileInput.click();
    return;
  }
  if (isUploadTooLarge(selectedFiles)) {
    alert(`文件超过上传限制，最大允许 ${MAX_UPLOAD_LABEL}`);
    return;
  }

  uploadBtn.disabled = true;
  uploadProgress.classList.remove("hidden");
  uploadBar.style.width = "2%";
  resultCard.classList.add("hidden");

  try {
    lastCode = await uploadWithProgress(selectedFiles);
    pickupCode.textContent = lastCode;
    resultCard.classList.remove("hidden");
    uploadBar.style.width = "100%";
    addHistoryRecord(lastCode, selectedFiles);
  } catch (error) {
    alert(`上传失败：${error.message || error}`);
  } finally {
    uploadBtn.disabled = false;
    setTimeout(resetUploadProgress, 450);
  }
});

copyBtn.addEventListener("click", async () => {
  if (!lastCode) return;
  try {
    await copyText(lastCode);
    copyBtn.textContent = "已复制";
  } catch (_) {
    copyBtn.textContent = "复制失败";
  }
  setTimeout(() => (copyBtn.textContent = "复制"), 1200);
});

codeInput.addEventListener("input", () => {
  codeInput.value = codeInput.value.replace(/\D/g, "").slice(0, 5);
});

codeInput.addEventListener("keydown", (event) => {
  if (event.key === "Enter") downloadBtn.click();
});

downloadBtn.addEventListener("click", async () => {
  const code = codeInput.value.trim();
  if (!/^\d{5}$/.test(code)) {
    setStatus("取件码格式错误", "请输入 5 位数字取件码", true);
    return;
  }

  downloadBtn.disabled = true;
  downloadProgress.classList.remove("hidden");
  downloadBar.style.width = "2%";
  setStatus("查询文件...", "正在连接服务器", false);

  try {
    const { invoke } = await loadTauriApi();
    const savedPath = await invoke("download_file", {
      server: getServer(),
      code,
      autoOpen: autoOpen.checked,
      autoUnzip: autoUnzip.checked,
    });
    downloadBar.style.width = "100%";
    setStatus("下载完成", savedPath, false);
  } catch (error) {
    setStatus("取件码不存在", String(error || "请检查取件码是否正确"), true);
  } finally {
    downloadBtn.disabled = false;
    setTimeout(resetDownloadProgress, 700);
  }
});

async function setupNativeEvents() {
  const { listen, invoke } = await loadTauriApi();

  unlistenDownloadProgress = await listen("download-progress", (event) => {
    const payload = event.payload || {};
    if (typeof payload.percent === "number") {
      downloadBar.style.width = `${Math.max(2, Math.min(100, payload.percent))}%`;
    }
    if (payload.title || payload.message) {
      setStatus(payload.title || "正在下载...", payload.message || "请稍候", false);
    }
  });

  autoStart.addEventListener("change", async () => {
    autoStart.disabled = true;
    try {
      await invoke("set_startup", { enabled: autoStart.checked });
    } catch (error) {
      alert(`设置开机启动失败：${error}`);
      autoStart.checked = !autoStart.checked;
    } finally {
      autoStart.disabled = false;
    }
  });
}

settingsBtn.addEventListener("click", openSettings);
cancelSettingsBtn.addEventListener("click", closeSettings);
settingsModal.addEventListener("click", (event) => {
  if (event.target === settingsModal) closeSettings();
});
serverInput.addEventListener("input", () => {
  testedServerUrl = "";
  saveServerBtn.disabled = true;
  setServerStatus("修改地址后，请先测试连接。");
});
testServerBtn.addEventListener("click", testServerConnection);
saveServerBtn.addEventListener("click", () => {
  if (!testedServerUrl) return;
  const saved = setServer(testedServerUrl);
  setServerStatus(`已保存并生效：${saved}`, "ok");
  setTimeout(closeSettings, 500);
});

clearHistoryBtn.addEventListener("click", () => {
  localStorage.removeItem(HISTORY_KEY);
  renderHistory();
});

window.addEventListener("beforeunload", () => {
  if (unlistenDownloadProgress) unlistenDownloadProgress();
});

window.setTimeout(() => {
  setupNativeEvents().catch(() => {});
}, 1500);

renderHistory();
