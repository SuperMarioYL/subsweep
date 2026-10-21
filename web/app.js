/* SubSweep 续费体检 — local UI logic. Vanilla JS, talks only to localhost. */
"use strict";

const $ = (sel) => document.querySelector(sel);

const signalLabels = {
  known_merchant: "命中服务名单",
  monthly_repeat: "按月重复扣款",
  keyword: "备注含代扣字样",
};
const statusLabels = { pending: "待确认", confirmed: "已确认", unrecognized: "不认识" };
const platformLabels = { alipay: "支付宝", wechat: "微信" };

let report = null;
let llmEnabled = false;
let uploadedFiles = [];

/* ---------- helpers ---------- */

function fmtFen(fen) {
  const neg = fen < 0 ? "-" : "";
  const abs = Math.abs(Number(fen));
  const yuan = Math.floor(abs / 100);
  const cents = String(abs % 100).padStart(2, "0");
  return neg + "¥" + yuan.toLocaleString("en-US") + "." + cents;
}

function el(tag, cls, text) {
  const node = document.createElement(tag);
  if (cls) node.className = cls;
  if (text !== undefined) node.textContent = text;
  return node;
}

async function api(path, body, method) {
  const opts = body
    ? { method: method || "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) }
    : { method: method || "GET" };
  const res = await fetch(path, opts);
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(data.error || res.statusText);
  return data;
}

/* ---------- data flow ---------- */

async function refresh() {
  const data = await api("/api/report");
  report = data.report;
  llmEnabled = data.llm_enabled;
  render();
}

async function upload(files) {
  const fd = new FormData();
  for (const f of files) fd.append("files", f, f.name);
  const res = await fetch("/api/bills", { method: "POST", body: fd });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    showUploadError(data.error || "上传失败");
    return;
  }
  uploadedFiles = uploadedFiles.concat(data.files || []);
  const bad = (data.files || []).filter((f) => f.error);
  if (bad.length) showUploadError(bad.map((f) => `${f.file}：${f.error}`).join("；"));
  else hideUploadError();
  await refresh();
}

function showUploadError(msg) {
  const p = $("#upload-error");
  p.textContent = msg;
  p.hidden = false;
}
function hideUploadError() { $("#upload-error").hidden = true; }

async function setStatus(item, status) {
  const data = await api("/api/status", { row_ids: item.row_ids, status });
  report = data.report;
  render();
}

async function pickService(item, canonicalName) {
  const data = await api("/api/pick", { merchant: item.raw_merchant, canonical_name: canonicalName });
  report = data.report;
  render();
}

async function suggestFor(item, container) {
  container.textContent = "识别中…";
  try {
    const data = await api("/api/suggest", { merchant: item.raw_merchant });
    container.textContent = "";
    const chips = el("div", "suggest-chips");
    for (const s of data.suggestions || []) {
      const label = s.source === "llm"
        ? `${s.canonical_name}（DeepSeek 判定）`
        : `${s.canonical_name}（匹配度 ${(s.score * 100).toFixed(0)}%）`;
      const btn = el("button", null, label);
      btn.type = "button";
      btn.addEventListener("click", () => pickService(item, s.canonical_name));
      chips.appendChild(btn);
    }
    if (!chips.childNodes.length) {
      container.appendChild(el("p", "suggest-error", "本地知识库没有匹配项。"));
    } else {
      container.appendChild(el("span", "suggest-error", "是下面哪个服务？点击替换："));
      container.appendChild(chips);
    }
    if (data.llm && data.llm.error) {
      container.appendChild(el("p", "suggest-error", "DeepSeek 辅助失败：" + data.llm.error));
    }
  } catch (err) {
    container.textContent = "";
    container.appendChild(el("p", "suggest-error", "识别失败：" + err.message));
  }
}

/* ---------- rendering ---------- */

function render() {
  const hasItems = report && report.items && report.items.length > 0;
  const hasData = report && report.transactions > 0;

  $("#summary").hidden = !hasData;
  $("#checklist").hidden = !hasData;
  for (const id of ["btn-export-md", "btn-export-html", "btn-export-json", "btn-reset"]) {
    $("#" + id).hidden = !hasData;
  }

  if (hasData) {
    $("#stat-items").textContent = report.items.length;
    $("#stat-annual").textContent = fmtFen(report.annualized_total_fen);
    $("#stat-forgotten").textContent = report.forgotten.length;
    renderItems();
  }
  renderFileList();
  renderLLMState();
}

function renderFileList() {
  const ul = $("#file-list");
  ul.textContent = "";
  for (const f of uploadedFiles) {
    const li = el("li");
    li.appendChild(el("span", "chip platform", platformLabels[f.platform] || f.platform || "?"));
    li.appendChild(el("span", null, f.file));
    if (f.error) {
      li.appendChild(el("span", "err", f.error));
    } else {
      li.appendChild(el("span", "chip", `${f.transactions} 笔交易`));
      li.appendChild(el("span", "chip signal", `疑似续费 ${f.suspects} 笔`));
    }
    ul.appendChild(li);
  }
}

function renderItems() {
  const wrap = $("#items");
  wrap.textContent = "";
  for (const item of report.items) wrap.appendChild(renderItem(item));
}

function renderItem(item) {
  const card = el("div", "item " + item.status);

  const head = el("div", "item-head");
  const name = el("div", "item-name");
  const title = el("h3", null, item.merchant);
  name.appendChild(title);
  name.appendChild(el("span", "chip platform", platformLabels[item.platform] || item.platform));
  if (item.raw_merchant && item.raw_merchant !== item.merchant) {
    name.appendChild(el("div", "raw-merchant", "账单商户：" + item.raw_merchant));
  }
  head.appendChild(name);

  const amounts = el("div", "item-amounts");
  amounts.append(
    el("b", null, fmtFen(item.amount_fen)), "/月",
    el("span", null, " · 年化 "),
    el("b", null, fmtFen(item.annualized_fen)),
    el("span", null,
      item.charges === 1
        ? ` · 扣款 1 笔（${item.first_date}）`
        : ` · 扣款 ${item.charges} 笔（${item.first_date} 至 ${item.last_date}）`),
  );
  head.appendChild(amounts);
  card.appendChild(head);

  const meta = el("div", "item-meta");
  for (const sig of item.signals || []) meta.appendChild(el("span", "chip signal", signalLabels[sig] || sig));
  meta.appendChild(el("span", "chip", `置信度 ${(item.confidence * 100).toFixed(0)}%`));
  meta.appendChild(el("span", "chip status-" + item.status, statusLabels[item.status] || item.status));
  card.appendChild(meta);

  card.appendChild(renderCancel(item));

  const actions = el("div", "item-actions");
  if (item.status === "confirmed") {
    const undo = el("button", "btn ghost small", "撤销确认");
    undo.type = "button";
    undo.addEventListener("click", () => setStatus(item, "pending"));
    actions.appendChild(undo);
  } else if (item.status === "unrecognized") {
    const undo = el("button", "btn ghost small", "撤销标记");
    undo.type = "button";
    undo.addEventListener("click", () => setStatus(item, "pending"));
    actions.appendChild(undo);
  } else {
    const ok = el("button", "btn small", "确认，我知道这笔");
    ok.type = "button";
    ok.addEventListener("click", () => setStatus(item, "confirmed"));
    const bad = el("button", "btn ghost small", "不认识这笔");
    bad.type = "button";
    bad.addEventListener("click", () => setStatus(item, "unrecognized"));
    actions.append(ok, bad);
    if (!item.service) {
      const guess = el("button", "btn ghost small", "这是什么服务？");
      guess.type = "button";
      const area = el("div", "suggest-area");
      area.hidden = true;
      guess.addEventListener("click", () => {
        if (area.hidden) { area.hidden = false; suggestFor(item, area); }
        else area.hidden = true;
      });
      actions.appendChild(guess);
      card.appendChild(area);
    }
  }
  card.appendChild(actions);
  return card;
}

function renderCancel(item) {
  const box = el("div", "cancel");
  if (item.service) {
    const c = item.service.cancel;
    const line = el("div", "steps");
    line.append(el("span", null, "官方取消路径："));
    const strong = el("strong", null, c.app + "：");
    line.appendChild(strong);
    line.append((c.steps || []).join(" → "));
    box.appendChild(line);
    if (c.url) {
      const link = el("a", null, c.url);
      link.href = c.url;
      link.target = "_blank";
      link.rel = "noopener noreferrer";
      box.appendChild(link);
      box.appendChild(el("br"));
    }
  } else {
    box.className = "cancel unknown";
    box.textContent = "知识库暂未收录该商户：请在其 App 内查找“自动续费管理”，或点击下方按钮尝试识别。";
  }
  return box;
}

function renderLLMState() {
  const p = $("#llm-state");
  p.textContent = llmEnabled
    ? "DeepSeek 辅助已开启（仅发送商户名，Key 只在本会话内存中）。"
    : "当前关闭：只使用本地字典与模糊匹配。";
  p.className = llmEnabled ? "hint ok" : "hint";
}

/* ---------- exports ---------- */

function download(name, mime, content) {
  const blob = new Blob([content], { type: mime });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = name;
  a.click();
  URL.revokeObjectURL(url);
}

function escapeHTML(s) {
  return String(s ?? "").replace(/[&<>"']/g, (c) => ({
    "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;",
  }[c]));
}

function exportHTML() {
  if (!report) return;
  const items = report.items.map((it, i) => {
    const cancel = it.service
      ? `${escapeHTML(it.service.cancel.app)}：${escapeHTML((it.service.cancel.steps || []).join(" → "))}` +
        (it.service.cancel.url ? `<br><a href="${escapeHTML(it.service.cancel.url)}" target="_blank" rel="noopener">${escapeHTML(it.service.cancel.url)}</a>` : "")
      : "知识库暂未收录该商户，请在其 App 内查找“自动续费管理”。";
    return `<li>
      <h3>${escapeHTML(it.merchant)}（${escapeHTML(platformLabels[it.platform] || it.platform)}）</h3>
      <p>月费 ${fmtFen(it.amount_fen)} ｜ 年化 ${fmtFen(it.annualized_fen)} ｜ 扣款 ${it.charges} 笔
        ｜ 信号：${escapeHTML((it.signals || []).map((s) => signalLabels[s] || s).join("、"))}
        ｜ 状态：${statusLabels[it.status] || it.status}</p>
      <p>${cancel}</p>
    </li>`;
  }).join("\n");
  const forgotten = report.forgotten.map((it) =>
    `<li>${escapeHTML(it.merchant)}（${statusLabels[it.status] || it.status}）— 年化 ${fmtFen(it.annualized_fen)}</li>`
  ).join("\n");
  const html = `<!DOCTYPE html>
<html lang="zh-CN"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>续费体检报告 · SubSweep</title>
<style>body{font-family:"PingFang SC","Microsoft YaHei",sans-serif;max-width:760px;margin:0 auto;padding:32px 20px;color:#1b2534;line-height:1.6}
h1{font-size:22px}h3{margin:14px 0 4px;font-size:15px}li{margin:14px 0;padding:12px 16px;border:1px solid #e2e8f0;border-radius:10px}
.total{font-size:18px;background:#e0f2f7;border-radius:10px;padding:12px 16px}
.muted{color:#64748b;font-size:13px}a{color:#0e7490;word-break:break-all}
footer{color:#94a3b8;font-size:12px;margin-top:32px}</style></head><body>
<h1>续费体检报告</h1>
<p class="muted">生成时间：${escapeHTML(report.generated_at)} · 扫描 ${report.statements} 份账单、${report.transactions} 笔交易</p>
<p class="total">疑似自动续费 ${report.items.length} 项，年化成本合计 <b>${fmtFen(report.annualized_total_fen)}</b>（月费 × 12，同一签约只计一次）</p>
${report.forgotten.length ? `<h2>疑似遗忘项（${report.forgotten.length}）</h2><ul>${forgotten}</ul>` : ""}
<h2>逐项明细</h2><ul>${items}</ul>
<footer>由 SubSweep 本地生成 · 检出结果供逐项核实，取消动作请由本人到官方页面完成</footer>
</body></html>`;
  download("subsweep-体检报告.html", "text/html;charset=utf-8", html);
}

function exportJSON() {
  if (!report) return;
  download("subsweep-report.json", "application/json;charset=utf-8", JSON.stringify(report, null, 2));
}

/* ---------- wiring ---------- */

function wire() {
  const zone = $("#dropzone");
  const input = $("#file-input");

  zone.addEventListener("click", () => input.click());
  zone.addEventListener("keydown", (e) => { if (e.key === "Enter" || e.key === " ") input.click(); });
  input.addEventListener("change", () => {
    if (input.files.length) upload([...input.files]);
    input.value = "";
  });
  for (const evt of ["dragenter", "dragover"]) {
    zone.addEventListener(evt, (e) => { e.preventDefault(); zone.classList.add("dragover"); });
  }
  for (const evt of ["dragleave", "drop"]) {
    zone.addEventListener(evt, (e) => { e.preventDefault(); zone.classList.remove("dragover"); });
  }
  zone.addEventListener("drop", (e) => {
    const files = [...(e.dataTransfer?.files || [])].filter((f) => /\.csv$/i.test(f.name) || f.type === "text/csv");
    if (files.length) upload(files);
    else showUploadError("请拖入 CSV 格式的账单导出文件。");
  });

  $("#btn-export-md").addEventListener("click", () => { location.href = "/api/export/report.md"; });
  $("#btn-export-html").addEventListener("click", exportHTML);
  $("#btn-export-json").addEventListener("click", exportJSON);
  $("#btn-reset").addEventListener("click", async () => {
    await api("/api/reset", {});
    uploadedFiles = [];
    hideUploadError();
    await refresh();
  });

  $("#llm-form").addEventListener("submit", async (e) => {
    e.preventDefault();
    const data = await api("/api/llm", { api_key: $("#llm-key").value.trim(), model: $("#llm-model").value.trim() });
    llmEnabled = data.enabled;
    $("#llm-key").value = "";
    renderLLMState();
  });
}

wire();
refresh();
