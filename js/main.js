(function () {
  var toggle = document.querySelector('.menu-toggle');
  var menu = document.getElementById('mobile-menu');
  var mobileLinks = menu ? menu.querySelectorAll('a') : [];
  var donateTriggers = document.querySelectorAll('[data-modal-open]');
  var modal = document.getElementById('donate-modal');
  var closeControls = modal ? modal.querySelectorAll('[data-modal-close]') : [];

  function closeMenu() {
    if (!toggle || !menu) {
      return;
    }

    menu.hidden = true;
    toggle.setAttribute('aria-expanded', 'false');
  }

  function openMenu() {
    if (!toggle || !menu) {
      return;
    }

    menu.hidden = false;
    toggle.setAttribute('aria-expanded', 'true');
  }

  if (toggle && menu) {
    toggle.addEventListener('click', function () {
      var expanded = toggle.getAttribute('aria-expanded') === 'true';

      if (expanded) {
        closeMenu();
      } else {
        openMenu();
      }
    });

    mobileLinks.forEach(function (link) {
      link.addEventListener('click', closeMenu);
    });
  }

  window.addEventListener('resize', function () {
    if (window.innerWidth >= 1024) {
      closeMenu();
    }
  });

  function openModal() {
    if (!modal) {
      return;
    }

    modal.hidden = false;
    document.body.classList.add('modal-open');
  }

  function closeModal() {
    if (!modal) {
      return;
    }

    modal.hidden = true;
    document.body.classList.remove('modal-open');
  }

  donateTriggers.forEach(function (trigger) {
    trigger.addEventListener('click', function () {
      closeMenu();
      openModal();
    });
  });

  closeControls.forEach(function (control) {
    control.addEventListener('click', closeModal);
  });

  document.addEventListener('keydown', function (event) {
    if (event.key === 'Escape') {
      closeMenu();
      closeModal();
    }
  });
})();