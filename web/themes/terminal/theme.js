// Terminal theme: the code shuffles through decoys and settles on the real
// one, then every line of .t-lines types itself in turn. Links are typed as
// "[Label]" and become the real links once their line is complete.
//
// The server renders the final state. While a line waits or types, its real
// nodes stay in the document inside a hidden holder, so i18n.js keeps
// updating them; each animation step only shows a plain-text copy. On every
// `ep:lang` the animation is reset to the final state and restarted from the
// current texts. When it ends the DOM equals the server-rendered one again.
// With prefers-reduced-motion: reduce this script changes nothing.
(function () {
  'use strict';

  var SHUFFLE_MS = 90;
  var SHUFFLE_STEPS = 8;
  var CHAR_MS = 18;
  var LINE_PAUSE_MS = 180;
  var DECOYS = ['200', '301', '302', '404', '410', '500', '418'];

  var body = document.body;
  var code = document.querySelector('.t-code');
  if (!body || !code) return;
  var target = code.textContent;
  var motion = window.matchMedia ? window.matchMedia('(prefers-reduced-motion: reduce)') : null;

  var run = 0; // bumped on every (re)start; stale timers check it and stop
  var timer = null;

  function lines() {
    return Array.prototype.slice.call(document.querySelectorAll('.t-lines > p'));
  }

  function removeClass(el, name) {
    el.classList.remove(name);
    if (!el.classList.length) el.removeAttribute('class');
  }

  function child(el, cls) {
    for (var n = el.firstElementChild; n; n = n.nextElementSibling) {
      if (n.classList.contains(cls)) return n;
    }
    return null;
  }

  // plain returns the visible text of a node, with links as "[Label]".
  function plain(node) {
    if (node.nodeType === Node.TEXT_NODE) return node.nodeValue;
    if (node.nodeType !== Node.ELEMENT_NODE || node.hidden) return '';
    var s = '';
    for (var n = node.firstChild; n; n = n.nextSibling) s += plain(n);
    return node.tagName === 'A' ? '[' + s + ']' : s;
  }

  // restore moves a line's real nodes back and drops the typing helpers.
  function restore(p) {
    var typed = child(p, 't-typed');
    if (typed) p.removeChild(typed);
    var hold = child(p, 't-hold');
    if (hold) {
      while (hold.firstChild) p.insertBefore(hold.firstChild, hold);
      p.removeChild(hold);
    }
    removeClass(p, 't-live');
  }

  function finish() {
    run++;
    if (timer !== null) clearTimeout(timer);
    timer = null;
    lines().forEach(restore);
    code.textContent = target;
    removeClass(body, 't-anim');
  }

  function later(id, fn, ms) {
    timer = setTimeout(function () {
      timer = null;
      if (id === run) fn();
    }, ms);
  }

  function start() {
    if (motion && motion.matches) return;
    finish();
    var id = run;

    // Clear every line: its nodes go into a hidden holder, its text is kept.
    var queue = [];
    lines().forEach(function (p) {
      if (p.hidden) return;
      var text = plain(p).replace(/\s+/g, ' ').trim();
      var hold = document.createElement('span');
      hold.className = 't-hold';
      hold.hidden = true;
      while (p.firstChild) hold.appendChild(p.firstChild);
      p.appendChild(hold);
      queue.push({ p: p, text: text });
    });
    body.classList.add('t-anim');

    var step = 0;
    function shuffle() {
      if (step < SHUFFLE_STEPS) {
        code.textContent = DECOYS[Math.floor(Math.random() * DECOYS.length)];
        step++;
        later(id, shuffle, SHUFFLE_MS);
        return;
      }
      code.textContent = target;
      typeLine(0);
    }

    function typeLine(i) {
      if (i >= queue.length) {
        finish();
        return;
      }
      var line = queue[i];
      // i18n.js may have replaced the line's content; it is final then.
      if (!child(line.p, 't-hold')) {
        line.p.classList.add('t-live');
        typeLine(i + 1);
        return;
      }
      var typed = document.createElement('span');
      typed.className = 't-typed';
      line.p.insertBefore(typed, line.p.firstChild);
      line.p.classList.add('t-live');
      var n = 0;
      function type() {
        n++;
        typed.textContent = line.text.slice(0, n);
        if (n < line.text.length) {
          later(id, type, CHAR_MS);
          return;
        }
        restore(line.p);
        line.p.classList.add('t-live');
        later(id, function () { typeLine(i + 1); }, LINE_PAUSE_MS);
      }
      later(id, type, CHAR_MS);
    }

    later(id, shuffle, SHUFFLE_MS);
  }

  document.addEventListener('ep:lang', function () {
    start();
    // A page opened in a background tab would animate under throttled
    // timers; show the final state at once.
    if (document.hidden) finish();
  });
  // A hidden tab throttles timers; show the final state instead of
  // resuming a half-typed page later.
  document.addEventListener('visibilitychange', function () {
    if (document.hidden) finish();
  });
  if (motion) {
    var onChange = function () { if (motion.matches) finish(); };
    if (motion.addEventListener) motion.addEventListener('change', onChange);
    else if (motion.addListener) motion.addListener(onChange);
  }
})();
