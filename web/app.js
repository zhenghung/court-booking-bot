let csrf = "";

function esc(s) {
  return String(s ?? "").replace(/[&<>"']/g, c => ({"&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;"}[c]));
}

async function api(path, opts = {}) {
  const res = await fetch(path, {
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", ...(csrf ? { "X-CSRF-Token": csrf } : {}), ...(opts.headers || {}) },
    ...opts,
  });
  const body = await res.json().catch(() => ({}));
  if (res.status === 401 && !document.getElementById("app").hidden) {
    location.reload();
    throw new Error("Session expired");
  }
  if (!res.ok) throw new Error(body.error || ("HTTP " + res.status));
  return body;
}

function show(view) {
  for (const b of document.querySelectorAll("nav.tabs button[data-view]")) {
    b.classList.toggle("active", b.dataset.view === view);
  }
  for (const s of ["dashboard", "schedules", "bookings", "probe", "book"]) {
    document.getElementById("view-" + s).hidden = s !== view;
  }
  if (view === "dashboard") loadStatus();
  if (view === "schedules") loadSchedules();
  if (view === "bookings") loadBookings();
}

function defaultDatePlus7() {
  const kl = new Date(Date.now() + 8 * 3600000 + 7 * 86400000);
  const m = String(kl.getUTCMonth() + 1).padStart(2, "0");
  const day = String(kl.getUTCDate()).padStart(2, "0");
  return `${kl.getUTCFullYear()}-${m}-${day}`;
}

async function loadStatus() {
  const box = document.getElementById("statusBox");
  try {
    const s = await api("/api/status");
    const hasSchedules = s.schedules && s.schedules.length;
    if (hasSchedules) {
      // Backend already sorts by nextRun and promotes hero to top-level.
      const hero = s.schedules[0];
      const heroAt = hero.nextRunAt || s.nextRunAt;
      document.getElementById("targetLine").textContent = `Next: ${hero.name} — ${hero.targetDay} · ${hero.nextRun || s.nextRun || ""}`;
      // Count same-night tie
      let tieCount = 0;
      if (heroAt) {
        const heroDay = new Date(heroAt).toISOString().slice(0,10);
        for (let i=1;i<s.schedules.length;i++) {
          const at = s.schedules[i].nextRunAt;
          if (at && new Date(at).toISOString().slice(0,10) === heroDay) tieCount++;
        }
      }
      let html = `<div class="runway-intro"><p class="hint">Cron daily 23:59 MYT — file-DB gated · ${s.schedules.length} schedules · Next booking date ${esc(s.targetDate || "—")} (${esc(s.targetDay || "—")})</p>`;
      if (tieCount) html += `<p class="hint">+${tieCount} more fire${tieCount>1?"s":""} same night as ${esc(hero.name)}</p>`;
      html += `</div>`;
      html += `<div class="runway">`;
      for (let idx=0; idx<s.schedules.length; idx++) {
        const v = s.schedules[idx];
        const isHero = idx===0;
        const slots = (v.bookingPlan||[]).map(p=>esc(p.slot)).join(", ") || "—";
        const courts = (v.bookingPlan||[]).flatMap(p=>p.courts||[]).join(", ") || "—";
        html += `<div class="runway-lane${isHero?" hero":""}">`
          + `<div class="runway-head"><span class="runway-name">${esc(v.name)}</span> <span class="sched-day">${esc(v.targetDay)}</span>${isHero?` <span class="runway-badge">FIRES NEXT</span>`:""}`
          + ` <span class="runway-next">${esc(v.nextRun || "")}</span></div>`
          + `<div class="runway-meta">${esc((v.accounts||[]).join(", "))} · ${slots} → ${esc(courts)}</div>`
          + `</div>`;
      }
      html += `</div>`;
      html += `<h3>Accounts</h3><ul>${(s.accounts || []).map(a => `<li>${esc(a.name)}</li>`).join("")}</ul>`;
      box.innerHTML = html;
      // Countdown to hero midnight, not +7 targetDate
      if (heroAt) startCountdownAt(heroAt);
      else if (hero.nextRun) startCountdown(s.targetDate);
      else startCountdown(s.targetDate);
    } else {
      document.getElementById("targetLine").textContent = `Target ${s.targetDay || ""} · ${s.targetDate || ""}`;
      let html = `<p>Target day: <strong>${esc(s.targetDay) || "—"}</strong></p>
        <p>Target date: <strong>${esc(s.targetDate) || "—"}</strong></p>
        <p>Next run: <strong>${esc(s.nextRun) || "—"}</strong></p>`;
      html += `<h3>Accounts</h3><ul>${(s.accounts || []).map(a => `<li>${esc(a.name)}</li>`).join("")}</ul>`;
      box.innerHTML = html;
      if (s.nextRunAt) startCountdownAt(s.nextRunAt);
      else startCountdown(s.targetDate);
    }
  } catch (e) { box.innerHTML = `<p class="error">${esc(e.message)}</p>`; }
}

function startCountdownAt(iso) {
  const el = document.getElementById("countdown");
  if (!iso) { el.textContent = "—"; return; }
  const midnight = new Date(iso).getTime();
  if (isNaN(midnight)) { el.textContent = "—"; return; }
  function tick() {
    const ms = midnight - Date.now();
    if (ms <= 0) { el.textContent = "window open"; clearInterval(startCountdownAt.t); return; }
    const h = Math.floor(ms / 3600000), m = Math.floor(ms % 3600000 / 60000), s2 = Math.floor(ms % 60000 / 1000);
    const d = Math.floor(h/24);
    if (d > 0) el.textContent = `fires in ${d}d ${String(h%24).padStart(2,"0")}:${String(m).padStart(2,"0")}:${String(s2).padStart(2,"0")}`;
    else el.textContent = `fires in ${String(h).padStart(2,"0")}:${String(m).padStart(2,"0")}:${String(s2).padStart(2,"0")}`;
  }
  tick();
  clearInterval(startCountdownAt.t);
  clearInterval(startCountdown.t);
  startCountdownAt.t = setInterval(tick, 1000);
}

function startCountdown(targetDate) {
  const el = document.getElementById("countdown");
  if (!targetDate) { el.textContent = "—"; return; }
  const [y, mo, d] = targetDate.split("-").map(Number);
  const midnight = Date.UTC(y, mo - 1, d) - 8 * 3600000;
  function tick() {
    const ms = midnight - new Date();
    if (ms <= 0) {
      el.textContent = "window open";
      clearInterval(startCountdown.t);
      clearInterval(startCountdownAt.t);
      return;
    }
    const h = Math.floor(ms / 3600000), m = Math.floor(ms % 3600000 / 60000), s2 = Math.floor(ms % 60000 / 1000);
    el.textContent = `opens in ${String(h).padStart(2, "0")}:${String(m).padStart(2, "0")}:${String(s2).padStart(2, "0")}`;
  }
  tick();
  clearInterval(startCountdown.t);
  clearInterval(startCountdownAt.t);
  startCountdown.t = setInterval(tick, 1000);
}

async function loadBookings() {
  const box = document.getElementById("bookingsBox");
  box.textContent = "Loading…";
  try {
    const data = await api("/api/bookings");
    box.innerHTML = (data.accounts || []).map(a =>
      `<h3>${esc(a.account)}</h3>` + (a.error ? `<p class="error">${esc(a.error)}</p>` :
        (a.bookings || []).map(b => `<p>${esc(b.date)} · ${esc(b.time)} · ${esc(b.facility)} · ${esc(b.status)}</p>`).join("") || "<p>No bookings.</p>")
    ).join("");
  } catch (e) { box.innerHTML = `<p class="error">${esc(e.message)}</p>`; }
}

async function probe() {
  const btn = document.getElementById("probeBtn");
  const box = document.getElementById("probeBox");
  const date = document.getElementById("probeDate").value || defaultDatePlus7();
  const courts = document.getElementById("probeCourts").value;
  btn.disabled = true;
  box.textContent = "Checking…";
  try {
    const data = await api(`/api/probe?date=${encodeURIComponent(date)}&courts=${encodeURIComponent(courts)}`);
    if (!data.courts || !data.courts.length) { box.textContent = "No courts."; return; }
    const times = [...new Set(data.courts.flatMap(c => (c.slots || []).map(s => s.time)))].sort();
    let html = `<table class="sheet"><tr><th>Time</th>${data.courts.map(c => `<th>${esc(c.name || c.id)}</th>`).join("")}</tr>`;
    for (const t of times) {
      html += `<tr><td><strong>${esc(t)}</strong></td>`;
      for (const c of data.courts) {
        const slot = (c.slots || []).find(s => s.time === t);
        if (!slot) { html += "<td>—</td>"; continue; }
        if (slot.available) {
          html += `<td><button class="slot available" data-time="${esc(t)}" data-court="${esc(c.name || c.id)}">Available — book</button></td>`;
        } else {
          html += `<td><span class="slot taken">Taken</span></td>`;
        }
      }
      html += "</tr>";
    }
    box.innerHTML = html + "</table>";
    for (const b of box.querySelectorAll("button.slot.available")) {
      b.onclick = () => {
        document.getElementById("bookDate").value = date;
        document.getElementById("bookTime").value = b.dataset.time;
        document.getElementById("bookCourt").value = b.dataset.court;
        refreshConfirmLabel();
        show("book");
      };
    }
  } catch (e) { box.innerHTML = `<p class="error">${esc(e.message)}</p>`; }
  finally { btn.disabled = false; }
}

async function book() {
  const btn = document.getElementById("bookBtn");
  const box = document.getElementById("bookBox");
  const payload = {
    date: document.getElementById("bookDate").value,
    time: document.getElementById("bookTime").value.trim(),
    facilityId: document.getElementById("bookCourt").value.trim(),
    dryRun: document.getElementById("bookDry").checked,
    confirm: document.getElementById("bookConfirm").checked,
  };
  btn.disabled = true;
  box.textContent = "Working…";
  try {
    const res = await api("/api/book", { method: "POST", body: JSON.stringify(payload) });
    box.innerHTML = `<p><strong>${res.dryRun ? "Would book" : "Booked"}:</strong> ${esc(res.message)}</p>`;
  } catch (e) { box.innerHTML = `<p class="error">${esc(e.message)}</p>`; }
  finally { btn.disabled = false; }
}

// --- Schedules ---
let schedCache = [];
let schedAccounts = [];
let schedFile = "";
let editingName = null;
let facilitiesCache = null;

async function loadSchedules() {
  const list = document.getElementById("schedList");
  const err = document.getElementById("schedError");
  err.textContent = "";
  list.textContent = "Loading…";
  try {
    const data = await api("/api/schedules");
    schedCache = data.schedules || [];
    schedAccounts = data.accounts || [];
    schedFile = data.scheduleFile || "";
    document.getElementById("schedFileHint").textContent = schedFile ? `File: ${schedFile}` : "No schedules file yet — saving will create schedules.yaml";
    if (!facilitiesCache) {
      try { const f = await api("/api/facilities"); facilitiesCache = f.facilities || []; } catch (_) { facilitiesCache = []; }
    }
    renderSchedList();
  } catch (e) { err.textContent = e.message; list.textContent = ""; }
}

function renderSchedList() {
  const list = document.getElementById("schedList");
  if (!schedCache.length) {
    list.innerHTML = `<p class="hint">No schedules. Create one.</p>`;
    return;
  }
  list.innerHTML = schedCache.map(s => {
    const slots = (s.bookingPlan || []).map(p => `${esc(p.slot)} → ${esc((p.courts||[]).join(", "))}`).join("<br>");
    return `<button class="sched-card${editingName===s.name?" active":""}" data-name="${esc(s.name)}">
      <div class="sched-name">${esc(s.name)} <span class="sched-day">${esc(s.targetDay)}</span></div>
      <div class="sched-meta">${esc(s.accounts||[].join(", "))} · ${esc(s.nextRun||"")}</div>
      <div class="sched-slot">${slots || "<span class='hint'>no slots</span>"}</div>
    </button>`;
  }).join("");
  for (const b of list.querySelectorAll(".sched-card")) b.onclick = () => openEditor(b.dataset.name);
}

function openEditor(name) {
  const existing = schedCache.find(s=>s.name===name);
  const editor = document.getElementById("schedEditor");
  editingName = name || null;
  document.getElementById("schedEditorTitle").textContent = existing ? `Edit ${name}` : "New schedule";
  document.getElementById("edName").value = existing ? existing.name : "";
  document.getElementById("edName").disabled = false;
  document.getElementById("edDay").value = existing ? existing.targetDay : "friday";
  document.getElementById("edError").textContent = "";
  document.getElementById("deleteSchedBtn").hidden = !existing;
  renderAccounts(existing ? existing.accounts : ["all"]);
  renderSlots(existing ? existing.bookingPlan : [{slot:"07:00-09:00", courts:[]}]);
  editor.hidden = false;
  renderSchedList();
  document.getElementById("edName").focus();
}

function renderAccounts(selected) {
  const wrap = document.getElementById("edAccounts");
  const sel = new Set(selected || []);
  const hasAll = sel.has("all");
  let html = `<label class="chip"><input type="checkbox" value="all" ${hasAll?"checked":""}> all</label>`;
  for (const a of schedAccounts) {
    const ck = !hasAll && sel.has(a);
    html += `<label class="chip"><input type="checkbox" value="${esc(a)}" ${ck?"checked":""} ${hasAll?"disabled":""}> ${esc(a)}</label>`;
  }
  if (!schedAccounts.length) html += `<span class="hint">no accounts from .env</span>`;
  wrap.innerHTML = html;
  const allCb = wrap.querySelector('input[value="all"]');
  if (allCb) allCb.onchange = () => renderAccounts(allCb.checked ? ["all"] : []);
  for (const cb of wrap.querySelectorAll('input[type="checkbox"]:not([value="all"])')) cb.onchange = () => {
    // if any individual checked while all checked, uncheck all
  };
}

function renderSlots(plan) {
  const wrap = document.getElementById("edSlots");
  wrap.innerHTML = "";
  (plan || []).forEach((entry, idx) => addSlotRow(entry.slot||"", entry.courts||[], wrap));
  if (!plan || !plan.length) addSlotRow("", [], wrap);
}

function addSlotRow(slot, courts, wrap) {
  if (!wrap) wrap = document.getElementById("edSlots");
  const row = document.createElement("div");
  row.className = "slot-row";
  const facOpts = (facilitiesCache||[]).map(f=>`<option value="${esc(f.name)}">${esc(f.name)} (${esc(f.id)})</option>`).join("");
  row.innerHTML = `
    <div class="slot-row-head">
      <input type="text" placeholder="07:00-09:00" value="${esc(slot)}" class="slot-input" style="max-width:160px">
      <button class="ghost" type="button" data-act="upSlot" title="Move up">↑</button>
      <button class="ghost" type="button" data-act="downSlot" title="Move down">↓</button>
      <button class="ghost" type="button" data-act="removeSlot">Remove slot</button>
    </div>
    <div class="chip-group court-chips"></div>
    <div class="court-add">
      <select class="court-select"><option value="">— pick court —</option>${facOpts}<option value="__custom">Custom…</option></select>
      <input type="text" class="court-custom" placeholder="Court name" style="display:none;flex:1;min-width:140px">
      <button class="ghost" type="button" data-act="addCourt">Add court</button>
    </div>`;
  const chips = row.querySelector(".court-chips");
  function drawChips() {
    chips.innerHTML = (row._courts||courts).map((c,i)=>`<span class="chip">${esc(c)} <button type="button" data-ci="${i}" title="Remove">×</button> <button type="button" data-mv="up" data-ci="${i}" title="Up">↑</button><button type="button" data-mv="down" data-ci="${i}" title="Down">↓</button></span>`).join("") || `<span class="hint">no courts — pick from list</span>`;
    for (const b of chips.querySelectorAll('button[data-ci]')) {
      if (b.dataset.mv) {
        b.onclick = () => {
          const arr = row._courts; const i=+b.dataset.ci;
          const j = b.dataset.mv==="up" ? i-1 : i+1;
          if (j<0||j>=arr.length) return; [arr[i],arr[j]]=[arr[j],arr[i]]; drawChips();
        };
      } else {
        b.onclick = () => { row._courts.splice(+b.dataset.ci,1); drawChips(); };
      }
    }
  }
  row._courts = [...courts];
  drawChips();
  const sel = row.querySelector(".court-select");
  const custom = row.querySelector(".court-custom");
  sel.onchange = () => { custom.style.display = sel.value==="__custom" ? "" : "none"; if (sel.value==="__custom") custom.focus(); };
  row.querySelector('[data-act="addCourt"]').onclick = () => {
    let v = sel.value;
    if (v==="__custom") v = custom.value.trim();
    if (!v || v==="__custom") return;
    if (row._courts.includes(v)) return;
    row._courts.push(v); drawChips(); sel.value=""; custom.value=""; custom.style.display="none";
  };
  row.querySelector('[data-act="removeSlot"]').onclick = () => row.remove();
  row.querySelector('[data-act="upSlot"]').onclick = () => { const p=row.previousElementSibling; if(p) row.parentNode.insertBefore(row,p); };
  row.querySelector('[data-act="downSlot"]').onclick = () => { const n=row.nextElementSibling; if(n) row.parentNode.insertBefore(n,row); };
  wrap.appendChild(row);
}

function collectEditorPayload() {
  const name = document.getElementById("edName").value.trim();
  const targetDay = document.getElementById("edDay").value;
  const accountCbs = [...document.querySelectorAll("#edAccounts input[type=checkbox]:checked")].map(cb=>cb.value);
  const accounts = accountCbs.length ? accountCbs : [];
  const bookingPlan = [...document.querySelectorAll("#edSlots .slot-row")].map(row=>{
    const slot = row.querySelector(".slot-input").value.trim();
    const courts = row._courts || [];
    return {slot, courts};
  }).filter(e=>e.slot||e.courts.length);
  return {name, targetDay, bookingPlan, accounts};
}

async function saveSchedule() {
  const err = document.getElementById("edError");
  err.textContent = "";
  const payload = collectEditorPayload();
  if (!payload.name) { err.textContent = "Name required (slug a-z0-9-)"; return; }
  if (!payload.bookingPlan.length) { err.textContent = "At least one slot required"; return; }
  for (const e of payload.bookingPlan) {
    if (!/^\d{2}:\d{2}-\d{2}:\d{2}$/.test(e.slot)) { err.textContent = `Invalid slot ${e.slot} (expected HH:MM-HH:MM)`; return; }
    if (!e.courts.length) { err.textContent = `Slot ${e.slot}: at least one court required`; return; }
  }
  if (!payload.accounts.length) { err.textContent = "Pick accounts or all"; return; }
  const btn = document.getElementById("saveSchedBtn");
  btn.disabled = true;
  try {
    const isEdit = editingName && schedCache.find(s=>s.name===editingName);
    if (isEdit) {
      await api(`/api/schedules/${encodeURIComponent(editingName)}`, {method:"PUT", body:JSON.stringify(payload)});
    } else {
      await api("/api/schedules", {method:"POST", body:JSON.stringify(payload)});
    }
    document.getElementById("schedEditor").hidden = true;
    editingName = null;
    await loadSchedules();
    await loadStatus();
  } catch(e){ err.textContent = e.message; }
  finally { btn.disabled = false; }
}

async function deleteSchedule() {
  if (!editingName) return;
  if (!confirm(`Delete schedule ${editingName}?`)) return;
  const err = document.getElementById("edError");
  err.textContent = "";
  try {
    await api(`/api/schedules/${encodeURIComponent(editingName)}`, {method:"DELETE"});
    document.getElementById("schedEditor").hidden = true;
    editingName = null;
    await loadSchedules();
    await loadStatus();
  } catch(e){ err.textContent = e.message; }
}

document.getElementById("newScheduleBtn").onclick = () => openEditor(null);
document.getElementById("refreshSchedulesBtn").onclick = loadSchedules;
document.getElementById("addSlotBtn").onclick = () => addSlotRow("", [], null);
document.getElementById("saveSchedBtn").onclick = saveSchedule;
document.getElementById("deleteSchedBtn").onclick = deleteSchedule;
document.getElementById("cancelSchedBtn").onclick = () => { document.getElementById("schedEditor").hidden = true; editingName = null; renderSchedList(); };

function enterApp() {
  document.getElementById("loginView").hidden = true;
  document.getElementById("app").hidden = false;
  document.getElementById("probeDate").value = defaultDatePlus7();
  document.getElementById("bookDate").value = defaultDatePlus7();
  refreshConfirmLabel();
  show("dashboard");
}

document.getElementById("loginBtn").onclick = async () => {
  const err = document.getElementById("loginErr");
  err.textContent = "";
  try {
    const res = await api("/api/login", { method: "POST", body: JSON.stringify({ password: document.getElementById("pw").value }) });
    csrf = res.csrfToken;
    document.getElementById("pw").value = "";
    enterApp();
  } catch (e) { err.textContent = e.message; }
};
document.getElementById("logoutBtn").onclick = async () => {
  await api("/api/logout", { method: "POST" }).catch(() => {});
  csrf = "";
  location.reload();
};
for (const b of document.querySelectorAll("nav.tabs button[data-view]")) b.onclick = () => show(b.dataset.view);
document.getElementById("probeBtn").onclick = probe;
document.getElementById("bookBtn").onclick = book;
function refreshConfirmLabel() {
  const court = document.getElementById("bookCourt").value.trim() || "best available";
  const time = document.getElementById("bookTime").value.trim() || "slot";
  const date = document.getElementById("bookDate").value || "date";
  document.getElementById("bookConfirmLabel").textContent = `Book ${court} ${time} on ${date} — I confirm live booking`;
}
for (const id of ["bookCourt", "bookTime", "bookDate"]) {
  document.getElementById(id).addEventListener("input", refreshConfirmLabel);
}
refreshConfirmLabel();

// Resume existing session (cookie survives reload) — skip login if valid.
(async () => {
  try {
    const s = await api("/api/session");
    csrf = s.csrfToken;
    enterApp();
  } catch (e) { /* stay on login */ }
})();
