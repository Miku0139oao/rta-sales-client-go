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
    const target = event.target as HTMLElement | null;
    const nested = target?.closest('.pane-scroll, .table-scroll, .facet-options, .store-grid');
    if (nested instanceof HTMLElement) {
      const canScroll = nested.scrollHeight > nested.clientHeight + 1;
      const style = getComputedStyle(nested);
      const scrollable = style.overflowY === 'auto' || style.overflowY === 'scroll';
      if (canScroll && scrollable) {
        if (event.deltaY < 0 && nested.scrollTop > 0) return;
        if (event.deltaY > 0 && nested.scrollTop + nested.clientHeight < nested.scrollHeight - 1) return;
      }
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
