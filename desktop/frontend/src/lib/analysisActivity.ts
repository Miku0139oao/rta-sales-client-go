import { writable } from "svelte/store";
export interface AnalysisActivity {
  operationId: string;
  phase: string;
  current: number;
  total: number;
  scope: string;
  startedAt: number;
  error?: string;
}
export const analysisActivity = writable<AnalysisActivity | undefined>(
  undefined,
);
