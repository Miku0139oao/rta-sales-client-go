import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

export default {
  preprocess: vitePreprocess(),
  compilerOptions: {
    warningFilter(warning) {
      const materialWebHostWarning = (
        warning.code === 'a11y_click_events_have_key_events' ||
        warning.code === 'a11y_no_static_element_interactions'
      ) && warning.message.includes('<md-');
      return !materialWebHostWarning;
    },
  },
  onwarn(warning, defaultHandler) {
    const materialWebHostWarning = (
      warning.code === 'a11y_click_events_have_key_events' ||
      warning.code === 'a11y_no_static_element_interactions'
    ) && warning.message.includes('<md-');
    if (!materialWebHostWarning) defaultHandler(warning);
  },
};
