// Copy text to the system clipboard. Returns a Promise<boolean> so
// callers can show success / failure UI. Two paths:
//   1. navigator.clipboard.writeText — modern, works on HTTPS and on
//      localhost over HTTP. Tried first.
//   2. execCommand('copy') fallback — for plain HTTP over LAN, where
//      clipboard API is rejected. The textarea is attached to
//      documentElement (above body) so it survives modal focus-trap
//      `inert` regions that would otherwise block selection / copy.
//      Selection state is also saved + restored so the user's text
//      selection inside the modal isn't clobbered by the copy.
export async function copyToClipboard(text) {
  if (text == null) return false;
  if (navigator.clipboard && window.isSecureContext) {
    try {
      await navigator.clipboard.writeText(String(text));
      return true;
    } catch (_) { /* fall through to execCommand path */ }
  }
  const ta = document.createElement('textarea');
  ta.value = String(text);
  ta.setAttribute('readonly', '');
  ta.style.position = 'fixed';
  ta.style.top = '0';
  ta.style.left = '0';
  ta.style.opacity = '0';
  ta.style.pointerEvents = 'none';
  // documentElement, not body — body becomes inert when a focus-trapped
  // modal is open, and inert ancestors prevent execCommand('copy') from
  // reading the selection.
  document.documentElement.appendChild(ta);
  const prevSelection = document.getSelection();
  const savedRanges = [];
  if (prevSelection) {
    for (let i = 0; i < prevSelection.rangeCount; i++) savedRanges.push(prevSelection.getRangeAt(i).cloneRange());
  }
  let ok = false;
  try {
    ta.focus({ preventScroll: true });
    ta.select();
    ok = document.execCommand('copy');
  } catch (_) { ok = false; }
  document.documentElement.removeChild(ta);
  if (prevSelection) {
    prevSelection.removeAllRanges();
    for (const r of savedRanges) prevSelection.addRange(r);
  }
  return ok;
}
