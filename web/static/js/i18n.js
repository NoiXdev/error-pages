// Shared page script: in-place language switch, Back/Reload actions and the
// local rendering of the maintenance end time. The page is complete without
// it; this only enhances.
(function () {
  'use strict';

  var root = document.documentElement;
  root.classList.add('ep-js');

  var texts = {};
  try {
    var el = document.getElementById('ep-texts');
    texts = JSON.parse(el ? el.textContent : '{}') || {};
  } catch (e) {
    texts = {};
  }
  var current = root.lang;
  var STORE = 'ep-lang';

  function t(lang, key) {
    var m = texts[lang];
    return m && Object.prototype.hasOwnProperty.call(m, key) ? m[key] : null;
  }

  function each(sel, fn) {
    Array.prototype.forEach.call(document.querySelectorAll(sel), fn);
  }

  function renderUntil(lang) {
    each('[data-until]', function (node) {
      var d = new Date(node.getAttribute('data-until'));
      var tpl = t(lang, 'ui.expected_back');
      if (isNaN(d.getTime()) || tpl === null) return;
      var when;
      try {
        when = new Intl.DateTimeFormat(lang, { dateStyle: 'medium', timeStyle: 'short' }).format(d);
      } catch (e) {
        when = d.toLocaleString();
      }
      node.textContent = tpl.split('{time}').join(when);
    });
  }

  function apply(lang) {
    if (!texts[lang]) return;
    current = lang;
    root.lang = lang;
    each('[data-t]', function (node) {
      var v = t(lang, node.getAttribute('data-t'));
      if (v !== null) node.textContent = v;
    });
    each('[data-t-label]', function (node) {
      var v = t(lang, node.getAttribute('data-t-label'));
      if (v !== null) node.setAttribute('aria-label', v);
    });
    each('[data-link]', function (node) {
      var href = t(lang, 'href.' + node.getAttribute('data-link'));
      if (href) node.setAttribute('href', href);
    });
    each('[data-lang]', function (node) {
      var on = node.getAttribute('data-lang') === lang;
      node.classList.toggle('on', on);
      if (on) node.setAttribute('aria-current', 'true');
      else node.removeAttribute('aria-current');
    });
    var title = t(lang, 'pages.' + root.getAttribute('data-page') + '.title');
    if (title !== null) document.title = root.getAttribute('data-code') + ' · ' + title;
    renderUntil(lang);
    document.dispatchEvent(new CustomEvent('ep:lang', { detail: { lang: lang } }));
  }

  function store(lang) {
    try { window.localStorage.setItem(STORE, lang); } catch (e) { /* storage unavailable */ }
  }

  function stored() {
    try { return window.localStorage.getItem(STORE); } catch (e) { return null; }
  }

  var params = new URLSearchParams(window.location.search);
  var saved = stored();
  if (!params.has('lang') && saved && saved !== current && texts[saved]) {
    apply(saved);
  } else {
    renderUntil(current);
  }

  document.addEventListener('click', function (ev) {
    if (ev.defaultPrevented || ev.button !== 0 || ev.metaKey || ev.ctrlKey || ev.shiftKey || ev.altKey) return;
    var a = ev.target.closest ? ev.target.closest('a') : null;
    if (!a) return;

    var lang = a.getAttribute('data-lang');
    if (lang && texts[lang]) {
      ev.preventDefault();
      store(lang);
      if (params.has('lang')) {
        params.set('lang', lang);
        try { history.replaceState(null, '', '?' + params.toString() + window.location.hash); } catch (e) { /* ignore */ }
      }
      if (lang !== current) apply(lang);
      return;
    }

    var action = a.getAttribute('data-action');
    if (action === 'back' && window.history.length > 1) {
      ev.preventDefault();
      window.history.back();
    } else if (action === 'reload') {
      ev.preventDefault();
      window.location.reload();
    }
  });
})();
