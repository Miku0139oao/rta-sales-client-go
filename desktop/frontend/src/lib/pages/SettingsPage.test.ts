import { cleanup, fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { translator } from '../i18n';
import { defaultSettings } from '../settings';
import SettingsPage from './SettingsPage.svelte';

afterEach(cleanup);

describe('settings unsaved changes', () => {
  it('disables save until a persistable field changes', async () => {
    const onChange = vi.fn();
    const onDirtyChange = vi.fn();
    const view = render(SettingsPage, {
      props: {
        t: translator('zh-TW'),
        settings: defaultSettings,
        onChange,
        onThemeChange: vi.fn(),
        onDirtyChange,
      },
    });
    const save = [...view.container.querySelectorAll('md-filled-button')].find((button) => button.textContent?.includes('儲存'));
    expect(save).toHaveAttribute('disabled');
    expect(screen.queryByText('有尚未儲存的變更')).not.toBeInTheDocument();

    await fireEvent.input(screen.getByRole('spinbutton', { name: '每次最多查詢工作' }), { target: { value: '17' } });
    expect(screen.getByText('有尚未儲存的變更')).toBeInTheDocument();
    expect(save).not.toHaveAttribute('disabled');
    expect(onDirtyChange).toHaveBeenCalledWith(true);

    await fireEvent.click(save!);
    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ maxJobs: 17, rankingLimit: defaultSettings.rankingLimit }));
    await view.rerender({
      t: translator('zh-TW'),
      settings: onChange.mock.calls[0][0],
      onChange,
      onThemeChange: vi.fn(),
      onDirtyChange,
    });
    expect(screen.getByText('設定已儲存')).toBeInTheDocument();
    expect(save).toHaveAttribute('disabled');
  });
});
