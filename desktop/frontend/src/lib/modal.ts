export interface ModalOptions {
  busy?: boolean;
  onClose: () => void;
}

export function modal(node: HTMLDialogElement, initial: ModalOptions) {
  let options = initial;
  const previouslyFocused = document.activeElement instanceof HTMLElement ? document.activeElement : undefined;

  function requestClose() {
    if (!options.busy) options.onClose();
  }

  function handleCancel(event: Event) {
    event.preventDefault();
    requestClose();
  }

  function handleBackdropClick(event: MouseEvent) {
    if (event.target !== node || options.busy) return;
    const bounds = node.getBoundingClientRect();
    const outside = bounds.width === 0 || bounds.height === 0 ||
      event.clientX < bounds.left || event.clientX > bounds.right ||
      event.clientY < bounds.top || event.clientY > bounds.bottom;
    if (outside) requestClose();
  }

  function handleWheel(event: WheelEvent) {
    let current = event.target as HTMLElement | null;
    while (current && node.contains(current)) {
      if (current.matches('.pane-scroll, .table-scroll, .facet-options, .store-grid, .export-dialog-scroll')) {
        const canScroll = current.scrollHeight > current.clientHeight + 1;
        const style = getComputedStyle(current);
        const scrollable = style.overflowY === 'auto' || style.overflowY === 'scroll';
        if (canScroll && scrollable) {
          if (event.deltaY < 0 && current.scrollTop > 0) return;
          if (event.deltaY > 0 && current.scrollTop + current.clientHeight < current.scrollHeight - 1) return;
        }
      }
      if (current === node) break;
      current = current.parentElement;
    }
    event.preventDefault();
    event.stopPropagation();
  }

  node.addEventListener('cancel', handleCancel);
  node.addEventListener('click', handleBackdropClick);
  node.addEventListener('wheel', handleWheel, { passive: false });

  try {
    if (typeof node.showModal === 'function') node.showModal();
    else node.setAttribute('open', '');
  } catch {
    node.setAttribute('open', '');
  }

  queueMicrotask(() => {
    const target = node.querySelector<HTMLElement>('[data-autofocus], input:not([disabled]), button:not([disabled]), [tabindex="0"]');
    target?.focus({ preventScroll: true });
  });

  return {
    update(next: ModalOptions) {
      options = next;
    },
    destroy() {
      node.removeEventListener('cancel', handleCancel);
      node.removeEventListener('click', handleBackdropClick);
      node.removeEventListener('wheel', handleWheel);
      if (node.open && typeof node.close === 'function') node.close();
      queueMicrotask(() => previouslyFocused?.focus({ preventScroll: true }));
    },
  };
}
