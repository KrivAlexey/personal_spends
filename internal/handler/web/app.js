// Upload page (ADR 0016). Builds every element with createElement and
// textContent: file names and server messages are caller input and must
// never be parsed as HTML (web-ui spec).
"use strict";

const byId = (id) => document.getElementById(id);
const STATUSES = new Set(["processing", "done", "failed"]);

function el(tag, text, className) {
  const node = document.createElement(tag);
  if (text !== undefined) node.textContent = text;
  if (className) node.className = className;
  return node;
}

function showMessage(node, text, kind) {
  node.textContent = text;
  node.className = "message " + kind;
  node.hidden = false;
}

async function loadBanks() {
  const select = byId("bank");
  const res = await fetch("/banks");
  if (!res.ok) throw new Error("GET /banks answered " + res.status);
  const { banks } = await res.json();
  select.replaceChildren(...banks.map((b) => {
    const option = el("option", b);
    option.value = b;
    return option;
  }));
}

function countCell(n) {
  return el("td", n === null ? "—" : String(n), "num");
}

function uploadRow(u) {
  const tr = document.createElement("tr");
  tr.append(el("td", u.filename === "" ? "(no name)" : u.filename, u.filename === "" ? "muted" : "file"));
  tr.append(el("td", u.bank));

  const when = el("td", new Date(u.uploaded_at).toLocaleString());
  when.title = u.uploaded_at;
  tr.append(when);

  const status = STATUSES.has(u.status) ? u.status : "unknown";
  const badge = el("span", u.status, "badge badge-" + status);
  const statusCell = document.createElement("td");
  statusCell.append(badge);
  tr.append(statusCell);

  tr.append(countCell(u.imported), countCell(u.already_known), countCell(u.pending_skipped));
  return tr;
}

async function loadUploads() {
  const table = byId("uploads");
  const empty = byId("empty");
  const listError = byId("list-error");
  try {
    const res = await fetch("/uploads");
    if (!res.ok) throw new Error("GET /uploads answered " + res.status + ": " + (await res.text()).trim());
    const { uploads } = await res.json();
    listError.hidden = true;
    byId("uploads-body").replaceChildren(...uploads.map(uploadRow));
    table.hidden = uploads.length === 0;
    empty.hidden = uploads.length !== 0;
  } catch (err) {
    showMessage(listError, "Could not load uploads. " + err.message, "error");
  }
}

async function upload(event) {
  event.preventDefault();
  const message = byId("message");
  const file = byId("file").files[0];
  if (!file) {
    showMessage(message, "Choose a CSV file first.", "error");
    return;
  }
  const button = byId("upload-button");
  button.disabled = true;
  button.textContent = "Uploading…";
  try {
    const params = new URLSearchParams({ bank: byId("bank").value, filename: file.name });
    // The file's bytes are the raw request body (ADR 0012), unchanged.
    const res = await fetch("/uploads/csv?" + params, {
      method: "POST",
      headers: { "Content-Type": "text/csv" },
      body: file,
    });
    if (res.ok) {
      const r = await res.json();
      showMessage(message,
        "Uploaded " + file.name + " (upload " + r.upload_id + "): " +
        r.imported + " imported, " + r.already_known + " already known, " +
        r.pending_skipped + " pending skipped.", "ok");
    } else {
      showMessage(message, "Error " + res.status + ": " + (await res.text()).trim(), "error");
    }
  } catch (err) {
    showMessage(message, "Upload failed: " + err.message, "error");
  } finally {
    button.disabled = false;
    button.textContent = "Upload";
    await loadUploads();
  }
}

document.addEventListener("DOMContentLoaded", () => {
  byId("upload-form").addEventListener("submit", upload);
  byId("refresh").addEventListener("click", loadUploads);
  loadBanks().catch((err) => showMessage(byId("message"), "Could not load banks. " + err.message, "error"));
  loadUploads();
});
