// Shared page script: in-place language switch, Back/Reload actions and the
// local rendering of the maintenance end time. The page is complete without
// it; this only enhances.
//
// The page inlines this file, followed by the theme's theme.js, in one
// script at the end of body, so the DOM is complete when it runs.
//
// Theme scripts: listen for the `ep:lang` event on document (detail.lang).
// It fires once on DOMContentLoaded, after the theme script has run, and
// again on each switch. At startup a theme script may also read
// document.documentElement.lang, which already holds the applied language.
(function () {
  'use strict';

  var root = document.documentElement;
  var texts = {};
  try {
    var el = document.getElementById('ep-texts');
    texts = JSON.parse(el ? el.textContent : '{}') || {};
  } catch (e) {
    texts = {};
  }
  var current = root.lang;
  var STORE = 'ep-lang';

  // t returns the text, or null when the key is absent.
  function t(lang, key) {
    var m = texts[lang];
    return m && Object.prototype.hasOwnProperty.call(m, key) ? m[key] : null;
  }

  function each(sel, fn) {
    Array.prototype.forEach.call(document.querySelectorAll(sel), fn);
  }

  // safeHref accepts site paths ("/x", not "//x") and http(s) URLs only.
  function safeHref(v) {
    if (typeof v !== 'string') return false;
    if (v.charAt(0) === '/') return v.charAt(1) !== '/' && v.charAt(1) !== '\\';
    return /^https?:\/\//i.test(v);
  }

  function renderUntil(lang) {
    each('[data-until]', function (node) {
      var d = new Date(node.getAttribute('data-until'));
      var tpl = t(lang, 'ui.expected_back');
      if (isNaN(d.getTime()) || !tpl) return;
      var when;
      try {
        when = new Intl.DateTimeFormat(lang, { dateStyle: 'medium', timeStyle: 'long' }).format(d);
      } catch (e) {
        when = d.toLocaleString();
      }
      node.textContent = tpl.split('{time}').join(when);
    });
  }

  function fire() {
    document.dispatchEvent(new CustomEvent('ep:lang', { detail: { lang: current } }));
  }

  function apply(lang) {
    if (!texts[lang]) return;
    current = lang;
    root.lang = lang;
    // Empty or absent texts hide their element, so nothing stale remains.
    each('[data-t]', function (node) {
      var v = t(lang, node.getAttribute('data-t'));
      node.textContent = v || '';
      node.hidden = !v;
    });
    each('[data-t-label]', function (node) {
      var v = t(lang, node.getAttribute('data-t-label'));
      if (v) node.setAttribute('aria-label', v);
    });
    each('[data-link]', function (node) {
      var href = t(lang, 'href.' + node.getAttribute('data-link'));
      if (safeHref(href)) node.setAttribute('href', href);
    });
    each('[data-lang]', function (node) {
      if (node.getAttribute('data-lang') === lang) {
        node.classList.add('on');
        node.setAttribute('aria-current', 'true');
      } else {
        // Drop the attribute rather than leave class="" behind.
        node.classList.remove('on');
        if (!node.classList.length) node.removeAttribute('class');
        node.removeAttribute('aria-current');
      }
    });
    var title = t(lang, 'pages.' + root.getAttribute('data-page') + '.title');
    if (title) document.title = root.getAttribute('data-code') + ' · ' + title;
    renderUntil(lang);
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
  document.addEventListener('DOMContentLoaded', fire);

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
      if (lang !== current) {
        apply(lang);
        fire();
      }
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
