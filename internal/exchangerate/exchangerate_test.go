package exchangerate

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeRepo struct {
	rates map[string]Rate
	err   error
}

func (f *fakeRepo) Find(ctx context.Context, currency string) (Rate, bool, error) {
	if f.err != nil {
		return Rate{}, false, f.err
	}
	rate, ok := f.rates[currency]
	return rate, ok, nil
}

func (f *fakeRepo) Save(ctx context.Context, rate Rate) error {
	if f.rates == nil {
		f.rates = map[string]Rate{}
	}
	f.rates[rate.Currency] = rate
	return nil
}

type fakeSource struct {
	rate Rate
	err  error
}

func (f fakeSource) Fetch(ctx context.Context, currency string) (Rate, error) {
	return f.rate, f.err
}

func withFakeECB(t *testing.T, status int, body string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	original := ecbURL
	ecbURL = server.URL
	t.Cleanup(func() { ecbURL = original })
}

const ecbDaily = `<?xml version="1.0" encoding="UTF-8"?>
<gesmes:Envelope xmlns:gesmes="http://www.gesmes.org/xml/2002-08-01" xmlns="http://www.ecb.int/vocabulary/2002-08-01/eurofxref">
	<gesmes:subject>Reference rates</gesmes:subject>
	<Cube>
		<Cube time='2026-10-13'>
			<Cube currency='USD' rate='1.0876'/>
			<Cube currency='JPY' rate='162.45'/>
		</Cube>
	</Cube>
</gesmes:Envelope>`

func TestECBClient_FetchParsesTheDailyRate(t *testing.T) {
	withFakeECB(t, http.StatusOK, ecbDaily)

	rate, err := NewECBClient().Fetch(context.Background(), USD)

	require.NoError(t, err)
	assert.Equal(t, USD, rate.Currency)
	assert.Equal(t, 1.0876, rate.PerEUR)
	assert.Equal(t, "2026-10-13", rate.Date.Format(time.DateOnly))
}

func TestECBClient_FetchFailsWithoutTheCurrency(t *testing.T) {
	withFakeECB(t, http.StatusOK, ecbDaily)

	_, err := NewECBClient().Fetch(context.Background(), "chf")

	require.Error(t, err)
}

func TestECBClient_FetchFailsOnAnErrorStatus(t *testing.T) {
	withFakeECB(t, http.StatusServiceUnavailable, "")

	_, err := NewECBClient().Fetch(context.Background(), USD)

	require.Error(t, err)
}

func TestRefresher_RefreshSavesTheFetchedRate(t *testing.T) {
	repo := &fakeRepo{}
	rate := Rate{Currency: USD, PerEUR: 1.0876, Date: time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)}

	require.NoError(t, NewRefresher(repo, fakeSource{rate: rate}, zap.NewNop()).Refresh(context.Background()))

	assert.Equal(t, rate, repo.rates[USD])
}

func TestRefresher_RefreshKeepsTheLastRateWhenTheSourceFails(t *testing.T) {
	previous := Rate{Currency: USD, PerEUR: 1.08}
	repo := &fakeRepo{rates: map[string]Rate{USD: previous}}

	err := NewRefresher(repo, fakeSource{err: errors.New("down")}, zap.NewNop()).Refresh(context.Background())

	require.Error(t, err)
	assert.Equal(t, previous, repo.rates[USD])
}

func TestService_USDPerEURIsZeroWithoutARate(t *testing.T) {
	rate, err := NewService(&fakeRepo{}).USDPerEUR(context.Background())

	require.NoError(t, err)
	assert.Zero(t, rate)
}

func TestService_USDPerEURReturnsTheSavedRate(t *testing.T) {
	repo := &fakeRepo{rates: map[string]Rate{USD: {Currency: USD, PerEUR: 1.0876}}}

	rate, err := NewService(repo).USDPerEUR(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 1.0876, rate)
}

func serve(t *testing.T, repo Repository) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewHandler(NewService(repo)).RegisterRoutes(router)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/exchange-rate", nil))
	return w
}

func TestHandler_GetReturnsTheRate(t *testing.T) {
	repo := &fakeRepo{rates: map[string]Rate{USD: {Currency: USD, PerEUR: 1.0876, Date: time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)}}}

	w := serve(t, repo)

	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"usd_per_eur": 1.0876, "date": "2026-10-13"}`, w.Body.String())
}

func TestHandler_GetReturnsNullsWithoutARate(t *testing.T) {
	w := serve(t, &fakeRepo{})

	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"usd_per_eur": null, "date": null}`, w.Body.String())
}

func TestHandler_GetFailsWhenTheRepositoryFails(t *testing.T) {
	w := serve(t, &fakeRepo{err: errors.New("db down")})

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
