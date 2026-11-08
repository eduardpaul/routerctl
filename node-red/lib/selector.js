'use strict';

// The event selector, the JavaScript twin of events.Selector in
// routerctl/internal/service/events: every non-empty field must match.

function normalizeMAC(s) {
  if (typeof s !== 'string') return '';
  const hex = s.trim().toLowerCase().replace(/[^0-9a-f]/g, '');
  if (hex.length !== 12 || !/^[0-9a-f:\-. ]+$/i.test(s.trim())) return '';
  return hex.match(/../g).join(':');
}

function list(v) {
  if (v === undefined || v === null || v === '') return [];
  const arr = Array.isArray(v) ? v : String(v).split(',');
  return arr.map((x) => String(x).trim()).filter(Boolean);
}

class Selector {
  constructor({ types, devices, macs, unknownOnly } = {}) {
    this.types = list(types);
    this.devices = list(devices);
    this.macs = list(macs).map((m) => normalizeMAC(m) || m.toLowerCase());
    this.unknownOnly = Boolean(unknownOnly);
  }

  match(e) {
    if (!e || typeof e !== 'object') return false;
    if (this.types.length && !this.types.includes(e.type)) return false;
    if (this.devices.length && !this.devices.includes(e.device)) return false;
    const data = e.data || {};
    if (this.macs.length) {
      const mac = normalizeMAC(data.mac);
      if (!mac || !this.macs.includes(mac)) return false;
    }
    if (this.unknownOnly && (typeof data.known !== 'boolean' || data.known)) return false;
    return true;
  }
}

module.exports = { Selector, normalizeMAC, list };
