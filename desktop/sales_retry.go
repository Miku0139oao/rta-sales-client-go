package desktop

import (
	"errors"
	"time"
)

type salesRetryState struct {
	id       string
	request  SalesAnalysisRequest
	stores   []selectedStore
	periods  []normalizedSalesAnalysisPeriod
	outcomes [][]storeOutcome
}

func (a *App) rememberSalesRetry(id string, request SalesAnalysisRequest, stores []selectedStore, periods []normalizedSalesAnalysisPeriod, outcomes [][]storeOutcome) {
	a.salesResultMu.Lock()
	defer a.salesResultMu.Unlock()
	if a.salesResult == nil || a.salesResult.OperationID != id || a.salesResult.Pending {
		return
	}
	a.salesRetry = &salesRetryState{id, request, stores, periods, outcomes}
}

// Retry only failed article-store/period and trend jobs; successful outcomes
// remain immutable inputs to the new aggregate. Credentials/routes are fresh.
func (a *App) RetrySalesAnalysis(request OperationRequest) (SalesAnalysisResult, error) {
	release, err := a.admitWork()
	if err != nil {
		return SalesAnalysisResult{}, err
	}
	defer release()
	ctx, finish, err := a.beginSalesAnalysisOperation(request.OperationID)
	if err != nil {
		return SalesAnalysisResult{}, err
	}
	defer finish()
	a.salesResultMu.Lock()
	state := a.salesRetry
	valid := state != nil && state.id == request.OperationID && a.salesResult != nil && a.salesResult.OperationID == request.OperationID && !a.salesResult.Pending
	a.salesResultMu.Unlock()
	if !valid {
		return SalesAnalysisResult{}, errors.New("retry context expired; run the analysis again / 重試資料已失效，請重新查詢")
	}
	ids := make([]string, len(state.stores))
	for i, store := range state.stores {
		ids[i] = store.route.store.BusinessID
	}
	selected, err := a.selectSalesAnalysisStores(ctx, state.request.ProfileID, ids, state.request.SimulateStoreCount)
	if err != nil {
		return SalesAnalysisResult{}, err
	}
	if len(selected) != len(state.stores) {
		return SalesAnalysisResult{}, errors.New("store permissions changed; rerun / 門店權限已變更，請重新查詢")
	}
	for i := range selected {
		if selected[i].route.store.BusinessID != ids[i] {
			return SalesAnalysisResult{}, errors.New("store scope changed; rerun / 門店範圍已變更，請重新查詢")
		}
	}
	if err := a.spreadAccountQuerySessions(selected, state.request.Concurrency, state.request.SimulateStoreCount); err != nil {
		return SalesAnalysisResult{}, err
	}
	jobs := []analysisJob{}
	for pi, outcomes := range state.outcomes {
		for si, outcome := range outcomes {
			if outcome.err != nil {
				kind := "article"
				if si == len(selected) {
					kind = "trend"
				}
				jobs = append(jobs, analysisJob{kind: kind, periodIndex: pi, storeIndex: si})
			}
		}
	}
	if len(jobs) == 0 {
		a.salesResultMu.Lock()
		defer a.salesResultMu.Unlock()
		return salesAnalysisForUpdate(*a.salesResult), nil
	}
	concurrency := state.request.Concurrency
	if concurrency < 1 {
		concurrency = 8
	}
	started := time.Now()
	run := a.startSalesAnalysisJobs(ctx, request.OperationID, selected, state.periods, nil, jobs, 0, len(jobs), concurrency, -1, state.outcomes)
	if err := run.wait(); err != nil {
		return SalesAnalysisResult{}, err
	}
	result, packed := assembleSalesAnalysisArticles(request.OperationID, selected, state.periods, run.finalOutcomes)
	result.Complete = len(result.Issues) == 0
	result.QueryDurationMS = time.Since(started).Milliseconds()
	result = a.rememberSalesAnalysisFor(state.request.ProfileID, result, packed)
	a.rememberSalesRetry(request.OperationID, state.request, selected, state.periods, run.finalOutcomes)
	a.events.Emit(a.appContext(), salesAnalysisUpdateEventName, result)
	return result, nil
}
