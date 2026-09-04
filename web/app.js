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
    location.reload(); // session expired — back to login
    throw new Error("Session expired");
  }
  if (!res.ok) throw new Error(body.error || ("HTTP " + res.status));
  return body;
}

function show(view) {
  for (const b of document.querySelectorAll("nav.tabs button[data-view]")) {
    b.classList.toggle("active", b.dataset.view === view);
  }
  for (const s of ["dashboard", "bookings", "probe", "book"]) {
    document.getElementById("view-" + s).hidden = s !== view;
  }
  if (view === "dashboard") loadStatus();
  if (view === "bookings") loadBookings();
}

function defaultDatePlus7() {
  // KL calendar date + 7, independent of browser timezone.
  const kl = new Date(Date.now() + 8 * 3600000 + 7 * 86400000);
  const m = String(kl.getUTCMonth() + 1).padStart(2, "0");
  const day = String(kl.getUTCDate()).padStart(2, "0");
  return `${kl.getUTCFullYear()}-${m}-${day}`;
}

async function loadStatus() {
  const box = document.getElementById("statusBox");
  try {
    const s = await api("/api/status");
    document.getElementById("targetLine").textContent = `Target ${s.targetDay || ""} · ${s.targetDate || ""}`;
    box.innerHTML = `<p>Target day: <strong>${esc(s.targetDay) || "—"}</strong></p>
      <p>Target date: <strong>${esc(s.targetDate) || "—"}</strong></p>
      <p>Next run: <strong>${esc(s.nextRun) || "—"}</strong></p>
      <h3>Accounts</h3><ul>${(s.accounts || []).map(a => `<li>${esc(a.name)}</li>`).join("")}</ul>`;
    startCountdown(s.targetDate);
  } catch (e) { box.innerHTML = `<p class="error">${esc(e.message)}</p>`; }
}

function startCountdown(targetDate) {
  const el = document.getElementById("countdown");
  if (!targetDate) { el.textContent = "—"; return; }
  // KL midnight (UTC+8) as an instant, correct in any browser timezone.
  const [y, mo, d] = targetDate.split("-").map(Number);
  const midnight = Date.UTC(y, mo - 1, d) - 8 * 3600000;
  function tick() {
    const ms = midnight - new Date();
    if (ms <= 0) {
      el.textContent = "window open";
      clearInterval(startCountdown.t);
      return;
    }
    const h = Math.floor(ms / 3600000), m = Math.floor(ms % 3600000 / 60000), s2 = Math.floor(ms % 60000 / 1000);
    el.textContent = `opens in ${String(h).padStart(2, "0")}:${String(m).padStart(2, "0")}:${String(s2).padStart(2, "0")}`;
  }
  tick();
  clearInterval(startCountdown.t);
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
