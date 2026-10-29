// Shared mobile navigation: injects a hamburger button into the page header
// and a full-screen overlay menu grouped by theme. The link list here is the
// single source of truth for the mobile menu on every page.
(function () {
  'use strict';

  var GROUPS = [
    { label: 'Samhälle', links: [
      ['/befolkning', 'Befolkning'],
      ['/utbildning', 'Skola'],
      ['/halsa', 'Folkhälsa'],
      ['/sjukskrivning', 'Sjukskrivning'],
    ]},
    { label: 'Ekonomi', links: [
      ['/ekonomi', 'Ekonomi & elpriser'],
      ['/inkomst', 'Inkomster'],
      ['/skatt', 'Skatt'],
      ['/arbetsmarknad', 'Arbetsmarknad'],
      ['/brp', 'Regional BNP'],
      ['/energi', 'Energi'],
    ]},
    { label: 'Politik & rätt', links: [
      ['/riksdagen', 'Riksdagen'],
      ['/val', 'Riksdagsval'],
      ['/polis', 'Polishändelser'],
      ['/brott', 'Anmälda brott'],
      ['/domstol', 'Domstolar'],
    ]},
    { label: 'Miljö & trafik', links: [
      ['/vader', 'Väder'],
      ['/trafik', 'Trafik & väglag'],
      ['/luftkvalitet', 'Luftkvalitet'],
    ]},
    { label: 'Övrigt', links: [
      ['/samband', 'Dolda samband'],
      ['/', 'Startsidan'],
      ['/om', 'Om projektet'],
    ]},
  ];

  var header = document.querySelector('header');
  if (!header) return;

  // ── Burger button ──────────────────────────────────
  var burger = document.createElement('button');
  burger.className = 'nav-burger';
  burger.setAttribute('aria-label', 'Öppna menyn');
  burger.setAttribute('aria-expanded', 'false');
  burger.setAttribute('aria-controls', 'nav-overlay');
  burger.innerHTML = '<i></i><i></i><i></i>';
  header.appendChild(burger);

  // ── Overlay ────────────────────────────────────────
  var overlay = document.createElement('div');
  overlay.className = 'nav-overlay';
  overlay.id = 'nav-overlay';
  overlay.setAttribute('role', 'dialog');
  overlay.setAttribute('aria-modal', 'true');
  overlay.setAttribute('aria-label', 'Meny');

  var path = location.pathname.replace(/\/$/, '') || '/';
  var html = '<button class="nav-close" aria-label="Stäng menyn">✕</button>';
  GROUPS.forEach(function (g) {
    html += '<div class="nav-group"><div class="nav-group-label">' + g.label + '</div><div class="nav-group-links">';
    g.links.forEach(function (l) {
      var active = l[0] === path ? ' class="active" aria-current="page"' : '';
      html += '<a href="' + l[0] + '"' + active + '>' + l[1] + '</a>';
    });
    html += '</div></div>';
  });
  html += '<p class="nav-overlay-foot">Sverige Nu · Öppen data · 21 län</p>';
  overlay.innerHTML = html;
  document.body.appendChild(overlay);

  // ── Open / close ───────────────────────────────────
  function setOpen(open) {
    overlay.classList.toggle('open', open);
    document.body.classList.toggle('nav-locked', open);
    burger.setAttribute('aria-expanded', String(open));
    burger.setAttribute('aria-label', open ? 'Stäng menyn' : 'Öppna menyn');
    if (open) overlay.querySelector('a').focus({ preventScroll: true });
  }

  burger.addEventListener('click', function () {
    setOpen(!overlay.classList.contains('open'));
  });
  overlay.querySelector('.nav-close').addEventListener('click', function () {
    setOpen(false);
    burger.focus();
  });
  document.addEventListener('keydown', function (e) {
    if (e.key === 'Escape' && overlay.classList.contains('open')) {
      setOpen(false);
      burger.focus();
    }
  });
})();
