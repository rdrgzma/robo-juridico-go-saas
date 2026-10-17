package integration

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rdrgzma/robo-juridico-go-saas/internal/admin"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/auth"
)

func TestHighConcurrencyQuotaExhaustionAndCapacity(t *testing.T) {
	f := identityDatabase(t)
	globalAdmin, _ := f.user(t, "load-admin", true)
	globalAdmin.MFAVerified = true

	adm := admin.New(f.pool, f.service, nil)

	// Create custom plan with strict quota limit of 25 units
	const quotaLimit = 25
	if err := adm.SavePlan(f.ctx, globalAdmin, admin.PlanInput{
		Code:              "plano_stress_test",
		Name:              "Plano Stress Test",
		MonthlyPrice:      "99.00",
		MonthlyLimit:      quotaLimit,
		RequestsPerMinute: 200,
		Features:          []string{"cnj", "liquidacao", "api"},
		Active:            true,
	}); err != nil {
		t.Fatal(err)
	}

	// Create client organization with limit of 25 units
	orgID, err := adm.CreateClientOrganization(f.ctx, globalAdmin, admin.CreateOrganizationInput{
		Slug:              "stress-client",
		Name:              "Stress Client LTDA",
		LegalName:         "Stress Test Sociedade",
		Document:          "55.444.333/0001-22",
		ContactName:       "Dr. Stress",
		ContactEmail:      "stress@test.org",
		OwnerEmail:        "owner@stress.org",
		OwnerPassword:     "SenhaValidaStress!2026",
		PlanCode:          "plano_stress_test",
		ContractedPrice:   "99.00",
		MonthlyLimit:      quotaLimit,
		RequestsPerMinute: 200,
		EffectiveFrom:     time.Now().AddDate(0, 0, -1).Format("2006-01-02"),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Issue API key
	key, rawKey, err := adm.IssueKey(f.ctx, globalAdmin, orgID, "Load Key", []string{"calculations:run"})
	if err != nil {
		t.Fatal(err)
	}
	_ = key

	// Launch 60 concurrent worker goroutines competing for 25 units
	const concurrentRequests = 60
	var wg sync.WaitGroup
	var successCount atomic.Int32
	var quotaDeniedCount atomic.Int32
	var otherErrors atomic.Int32

	startBarrier := make(chan struct{})
	start := time.Now()

	for i := 0; i < concurrentRequests; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			<-startBarrier // synchronize start for maximal concurrency

			res, err := f.service.BeginAPI(f.ctx, rawKey, "calculations:run", "127.0.0.1", true)
			if err == nil {
				successCount.Add(1)
				// Complete transaction successfully
				_ = f.service.FinishAPI(f.ctx, res.EventID, 200)
			} else if errors.Is(err, auth.ErrQuota) {
				quotaDeniedCount.Add(1)
			} else {
				otherErrors.Add(1)
			}
		}(i)
	}

	close(startBarrier)
	wg.Wait()
	elapsed := time.Since(start)

	// Validate exact quota enforcement under high concurrency:
	// Exactly 25 requests must have succeeded, and remaining 35 must have been denied with ErrQuota.
	succeeded := successCount.Load()
	denied := quotaDeniedCount.Load()
	others := otherErrors.Load()

	if others > 0 {
		t.Fatalf("unexpected errors during concurrent load: %d", others)
	}
	if succeeded != quotaLimit {
		t.Fatalf("race condition in quota counter: expected exactly %d succeeded, got %d", quotaLimit, succeeded)
	}
	if denied != (concurrentRequests - quotaLimit) {
		t.Fatalf("expected exactly %d quota denials, got %d", concurrentRequests-quotaLimit, denied)
	}

	// Verify database persisted quota state matches perfectly
	var used, blocked int
	err = f.pool.QueryRow(f.ctx, `SELECT used, blocked FROM quota_periods WHERE organization_id=$1`, orgID).Scan(&used, &blocked)
	if err != nil {
		t.Fatal(err)
	}
	if used != quotaLimit {
		t.Fatalf("persisted quota used mismatch: expected %d, got %d", quotaLimit, used)
	}
	if blocked != int(denied) {
		t.Fatalf("persisted quota blocked mismatch: expected %d, got %d", denied, blocked)
	}

	t.Logf("Concurrency load test: %d requests in %v (avg %.2fms/req) -> exactly %d accepted, %d rejected",
		concurrentRequests, elapsed, float64(elapsed.Milliseconds())/float64(concurrentRequests), succeeded, denied)
}
