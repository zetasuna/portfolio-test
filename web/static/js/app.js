'use strict';

document.addEventListener('submit', (event) => {
  const form = event.target;
  if (form instanceof HTMLFormElement && form.dataset.confirm) {
    if (!window.confirm(form.dataset.confirm)) event.preventDefault();
  }
});

// Keep the visual state obvious for keyboard users on forms and navigation.
document.addEventListener('keydown', (event) => {
  if (event.key === 'Escape') {
    document.activeElement instanceof HTMLElement && document.activeElement.blur();
  }
});
