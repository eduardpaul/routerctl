'use strict';

// Server-Sent Events parser (the text/event-stream format of the WHATWG
// HTML spec): feed it text chunks, it calls onEvent({event, id, data})
// for every dispatched event. Comments (": ping") are ignored, "retry:"
// is reported through onRetry. No dependencies.

class SSEParser {
  constructor({ onEvent, onRetry } = {}) {
    this.onEvent = onEvent || (() => {});
    this.onRetry = onRetry || (() => {});
    this.buf = '';
    this.reset();
    // The last event id survives dispatches (spec: "last event ID buffer")
    this.lastId = '';
  }

  reset() {
    this.event = '';
    this.data = [];
    this.id = undefined;
  }

  feed(chunk) {
    this.buf += chunk;
    for (;;) {
      const m = /\r\n|\r|\n/.exec(this.buf);
      if (!m) break;
      // A lone \r at the very end may be the first half of \r\n
      if (m[0] === '\r' && m.index === this.buf.length - 1) break;
      const line = this.buf.slice(0, m.index);
      this.buf = this.buf.slice(m.index + m[0].length);
      this.line(line);
    }
  }

  line(line) {
    if (line === '') {
      this.dispatch();
      return;
    }
    if (line.startsWith(':')) return;
    const i = line.indexOf(':');
    let field = line;
    let value = '';
    if (i >= 0) {
      field = line.slice(0, i);
      value = line.slice(i + 1);
      if (value.startsWith(' ')) value = value.slice(1);
    }
    switch (field) {
      case 'event':
        this.event = value;
        break;
      case 'data':
        this.data.push(value);
        break;
      case 'id':
        if (!value.includes('\0')) this.id = value;
        break;
      case 'retry':
        if (/^\d+$/.test(value)) this.onRetry(Number(value));
        break;
      default:
    }
  }

  dispatch() {
    if (this.id !== undefined) this.lastId = this.id;
    if (this.data.length === 0) {
      this.reset();
      return;
    }
    const ev = { event: this.event || 'message', id: this.lastId, data: this.data.join('\n') };
    this.reset();
    this.onEvent(ev);
  }
}

module.exports = { SSEParser };
