package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gamepanel/forge/internal/services/activity"

	"github.com/gofiber/fiber/v2"
)

type fakeActivityStore struct {
	events []activity.ActivityEvent
	filter activity.ActivityFilter
}

func (s *fakeActivityStore) InsertActivity(context.Context, *activity.ActivityEvent) error {
	return nil
}

func (s *fakeActivityStore) QueryActivities(_ context.Context, filter activity.ActivityFilter) ([]activity.ActivityEvent, error) {
	s.filter = filter
	return s.events, nil
}

func (s *fakeActivityStore) CountActivities(context.Context, activity.ActivityFilter) (int, error) {
	return len(s.events), nil
}

func (s *fakeActivityStore) CleanupActivities(context.Context, time.Time) (int64, error) {
	return 0, nil
}

func (s *fakeActivityStore) GetActivityStats(context.Context) (*activity.ActivityStats, error) {
	return &activity.ActivityStats{}, nil
}

func TestAdminActivityRoutesAreCanonicalAndProtected(t *testing.T) {
	store := &fakeActivityStore{events: []activity.ActivityEvent{{ID: "event-1"}}}
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("user", tokenClaims{Role: "admin"})
		return c.Next()
	})
	registerActivityRoutes(app, Config{ActivityService: activity.New(store)})

	request := httptest.NewRequest(http.MethodGet, "/admin/activity?actorId=user-1&subjectType=server", nil)
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET /admin/activity status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	if store.filter.ActorID == nil || *store.filter.ActorID != "user-1" {
		t.Fatalf("actor filter = %#v, want user-1", store.filter.ActorID)
	}
	if store.filter.SubjectType == nil || *store.filter.SubjectType != "server" {
		t.Fatalf("subject type filter = %#v, want server", store.filter.SubjectType)
	}

	exportRequest := httptest.NewRequest(http.MethodGet, "/admin/activity/export?actorId=user-2&subjectId=server-1&offset=10", nil)
	exportResponse, err := app.Test(exportRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer exportResponse.Body.Close()
	if exportResponse.StatusCode != http.StatusOK {
		t.Fatalf("GET /admin/activity/export status = %d, want %d", exportResponse.StatusCode, http.StatusOK)
	}
	if store.filter.ActorID == nil || *store.filter.ActorID != "user-2" || store.filter.SubjectID == nil || *store.filter.SubjectID != "server-1" {
		t.Fatalf("export filter = %#v, want actor and subject filters", store.filter)
	}
	if store.filter.Limit != 200 || store.filter.Offset != 0 {
		t.Fatalf("export pagination = limit %d, offset %d; want limit 200, offset 0", store.filter.Limit, store.filter.Offset)
	}

	legacyGlobalRequest := httptest.NewRequest(http.MethodGet, "/activity/events", nil)
	legacyGlobalResponse, err := app.Test(legacyGlobalRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer legacyGlobalResponse.Body.Close()
	if legacyGlobalResponse.StatusCode != http.StatusNotFound {
		t.Fatalf("GET /activity/events status = %d, want %d", legacyGlobalResponse.StatusCode, http.StatusNotFound)
	}
}

func TestCurrentUserActivityRouteIsAccountScoped(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("user", tokenClaims{Role: "user"})
		return c.Next()
	})
	registerAuthRoutes(app, Config{}, func(c *fiber.Ctx) error { return c.Next() })
	registerActivityRoutes(app, Config{ActivityService: activity.New(&fakeActivityStore{})})

	accountResponse, err := app.Test(httptest.NewRequest(http.MethodGet, "/account/activity", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer accountResponse.Body.Close()
	if accountResponse.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("GET /account/activity status = %d, want %d when no store is configured", accountResponse.StatusCode, http.StatusServiceUnavailable)
	}

	removedResponse, err := app.Test(httptest.NewRequest(http.MethodGet, "/activity", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer removedResponse.Body.Close()
	if removedResponse.StatusCode != http.StatusNotFound {
		t.Fatalf("GET /activity status = %d, want %d", removedResponse.StatusCode, http.StatusNotFound)
	}
}

func TestAdminActivityQueryRejectsNonAdmin(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("user", tokenClaims{Role: "user"})
		return c.Next()
	})
	registerActivityRoutes(app, Config{ActivityService: activity.New(&fakeActivityStore{})})

	response, err := app.Test(httptest.NewRequest(http.MethodGet, "/admin/activity", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("GET /admin/activity status = %d, want %d", response.StatusCode, http.StatusForbidden)
	}
}

func TestCSVSafeRecordEscapesFormulaPrefixes(t *testing.T) {
	record := csvSafeRecord([]string{"=sum(A1:A2)", "+1", "-1", "@name", "plain", "  =not-a-formula", ""})
	want := []string{"'=sum(A1:A2)", "'+1", "'-1", "'@name", "plain", "  =not-a-formula", ""}
	for i := range want {
		if record[i] != want[i] {
			t.Fatalf("record[%d] = %q, want %q", i, record[i], want[i])
		}
	}
}
