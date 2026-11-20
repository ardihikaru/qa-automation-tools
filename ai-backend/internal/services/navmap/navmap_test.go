package navmap

import (
	"math/rand"
	"reflect"
	"strings"
	"testing"
)

func TestNormPath(t *testing.T) {
	cases := map[string]string{
		"/report-builder/list":                          "/report-builder/list",
		"/report-builder/list/":                         "/report-builder/list",
		"/report-builder/list?x=1#top":                  "/report-builder/list",
		"https://stg.example.id/api/v1/x?token=1":       "/api/v1/x",
		"stg.example.id:8443/api/v1/x":                  "/api/v1/x",
		"127.0.0.1:8765/emr/patient":                    "/emr/patient",
		"/swagger/<relativePath>":                       "/swagger/:relativePath",
		"/patients/{idType}/{idValue}":                  "/patients/:idType/:idValue",
		"/chart-management/db/${chartDashboardID}/edit": "/chart-management/db/:param/edit",
		"/x/${encodeURIComponent(a)}":                   "/x/:param",
		"/kiosk-sut-v2/patients/123":                    "/kiosk-sut-v2/patients/:id",
		"/a/0f8fad5b-d9cb-469f-a165-70867728950e":       "/a/:id",
		"/a/0123456789abcdef0":                          "/a/:id",
		"//a//b":                                        "/a/b",
		"/api/v1/swagger/*":                             "/api/v1/swagger/*",
		"/view/:slug":                                   "/view/:slug",
		"/":                                             "/",
		"/api/v1/../etc/passwd":                         "",
		"/<path>":                                       "",
		"/*":                                            "",
		"relative":                                      "",
		"/a b":                                          "",
		"/a`b":                                          "",
		"/" + strings.Repeat("a", MaxPathLen):           "",
	}
	for in, want := range cases {
		if got := NormPath(in); got != want {
			t.Errorf("NormPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMatch(t *testing.T) {
	for _, c := range []struct {
		pattern, path string
		ok            bool
		literal       int
	}{
		{"/a/:id", "/a/:id", true, 1},
		{"/a/:id", "/a/7", true, 1},
		{"/a/b", "/a/b", true, 2},
		{"/a/b", "/a/c", false, 0},
		{"/a/*", "/a/b/c", true, 1},
		{"/a/*", "/a", false, 0},
		{"/a", "/a/b", false, 0},
	} {
		ok, lit := Match(c.pattern, c.path)
		if ok != c.ok || (ok && lit != c.literal) {
			t.Errorf("Match(%q, %q) = %v %d, want %v %d", c.pattern, c.path, ok, lit, c.ok, c.literal)
		}
	}
}

// Files shaped like the SUT automation repo (NAVIGATION_MAP.md 3.3).
var repoFiles = map[string]string{
	"src/web/shared/base.page.ts": `export abstract class BasePage {
  protected abstract readonly path: string;
}`,
	"src/web/sut-frontend/pages/auth/login.page.ts": `export class LoginPage extends BasePage {
  protected readonly path: string = '/auth/login';
}

export class ImpersonatedLoginPage extends LoginPage {
  protected override readonly path = '/auth/impersonated-login';
}

export class NoPathPage extends LoginPage {
}`,
	"src/web/sut-frontend/pages/report-builder/report-list.page.ts": `export class ReportBuilderListPage extends BasePage {
  protected readonly path = '/report-builder/list';
}`,
	"src/web/sut-frontend/pages/chart/dashboard-builder.page.ts": `export class DashboardBuilderPage extends BasePage {
  protected readonly path = '/chart-management/db/new';
  async open(id: string) {
    await this.page.goto(` + "`/chart-management/db/${id}/edit`" + `);
  }
}`,
	"src/web/sut-frontend/sut-frontend.app.ts": `export class SutFrontendApp {
  readonly page: Page;
  readonly loginPage: LoginPage;
  readonly impersonatedLoginPage: ImpersonatedLoginPage;
}`,
	"src/fixtures/web.fixtures.ts": `export const webTest = base.extend<WebFixtures>({
  hisFrontend: async ({ page }, use) => {
    await use(new SutFrontendApp(page));
  },

  reportBuilderList: async ({ page }, use) => {
    await use(new ReportBuilderListPage(page));
  },
});`,
	"tests/web/sut-frontend/auth/login.spec.ts": `test.describe('login', () => {
  test('shows a validation error', async ({ hisFrontend }) => {
    await hisFrontend.loginPage.goto();
  });
  test('TC-AUTH-020 impersonated login', async ({ hisFrontend }) => {
    await hisFrontend.impersonatedLoginPage.goto();
  });
});`,
	"tests/web/sut-frontend/report-builder/list.spec.ts": `test.describe('list', () => {
  test(
    'TC-RB-060: list page loads',
    async ({ hisFrontend, reportBuilderList }) => {
      await reportBuilderList.goto();
    },
  );
});`,
	"src/api/rest/sut-vendor-backend/sut-vendor-backend.client.ts": `/** Every ` + "`POST /omnichannel/<operation>`" + ` endpoint. */
export class SutVendorBackendClient extends BaseApiClient {
  constructor(request: APIRequestContext) {
    super(request, baseURL);
  }

  /** Shared transport. */
  private call(operation: string): Promise<APIResponse> {
    return this.request.fetch(this.url(` + "`/omnichannel/${operation}`" + `), { method: 'POST' });
  }

  /** ` + "`POST /omnichannel/booking`" + ` — create a booking. */
  booking(body?: Body): Promise<APIResponse> {
    return this.call('booking', body);
  }

  /** ` + "`GET|POST|DELETE /service-status`" + ` — the liveness probe. */
  serviceStatus(method: 'GET' | 'POST' | 'DELETE' = 'GET'): Promise<APIResponse> {
    return this.get('service-status');
  }

  /** ` + "`GET /swagger/<relativePath>`" + ` — docs. */
  swagger(relativePath: string): Promise<APIResponse> {
    return this.get(` + "`swagger/${relativePath}`" + `);
  }

  /** Probe a route that does not exist. ` + "`GET /<path>`" + ` */
  probeUnknownRoute(path: string): Promise<APIResponse> {
    return this.get(path);
  }

  vtt(): Promise<APIResponse> {
    return this.get(
      'vtt/patients',
    );
  }
}`,
	"tests/api/rest/sut-vendor-backend/TC-VSYS-001.spec.ts": `test('TC-VSYS-001 the liveness probe answers 200', async ({ vendorApi }) => {
  const response = await vendorApi.serviceStatus();
});`,
	"tests/api/rest/sut-vendor-backend/kiosk-patient-whatsapp.spec.ts": `test('TC-PATREG-070 kiosk registers a patient', async ({ request }) => {
  const response = await request.post(` + "`${env.SUT_VENDOR_BACKEND_URL}/kiosk-sut-v2/patients`" + `, {});
});`,
	"docs/ignored.ts": `readonly path = '/never'`,
}

func testInput() Input {
	files := map[string][]byte{}
	for k, v := range repoFiles {
		files[k] = []byte(v)
	}
	return Input{
		Features: []Feature{
			{Code: "B06.01", Name: "`GET /service-status` tanpa auth", RepoEntry: "sut-vendor-backend `/api/v1/service-status`"},
			{Code: "B06.02", Name: "Swagger UI `/swagger/*`", RepoEntry: "sut-vendor-backend `/api/v1/swagger/*`"},
			{Code: "B01.01", Name: "Kiosk", RepoEntry: "sut-vendor-backend `/kiosk-sut-v2`"},
			{Code: "A01.01", Name: "Pencarian pasien (`/master-patient`)", RepoEntry: "sut-backend + sut-frontend"},
			{Code: "Z01.01", Name: "No route", RepoEntry: "belum ada"},
		},
		Epics: []Epic{{Code: "VSYS", Service: "vendor-backend"}, {Code: "VKAUTH", Service: "vendor-backend"}, {Code: "RB", Service: "sut-backend"}, {Code: "AUTH", Service: "sut-backend"}},
		Stories: []Story{
			{Code: "US-VSYS-01", FeatureCodes: []string{"B06.01"}},
			{Code: "US-PATREG-01", FeatureCodes: []string{"A01.01"}},
		},
		Cases: []Case{
			{Code: "TC-VSYS-001", EpicCode: "VSYS", StoryCode: "US-VSYS-01", Polarity: "+", Scenario: "Given the service is up, when `GET /api/v1/service-status` is sent, then 200", SpecPaths: []string{"tests/api/rest/sut-vendor-backend/TC-VSYS-001.spec.ts"}},
			{Code: "TC-VSYS-003", EpicCode: "VSYS", Polarity: "-", Scenario: "Given the route `/api/v1/service-status` exists for GET only, when `POST /api/v1/service-status` is sent, then 405"},
			{Code: "TC-VSYS-006", EpicCode: "VSYS", Polarity: "-", Scenario: "Given an unregistered path, when `GET /api/v1/thisisnotaroute` is sent, then 404 (source lists `GET /service-status`)"},
			{Code: "TC-VKAUTH-007", EpicCode: "VKAUTH", Polarity: "+", Scenario: "`V-KIOSK-AUTH-public-OK-01` GET /kiosk-sut-v2/health — GET /health tanpa credential → 200"},
			{Code: "TC-VKAUTH-012", EpicCode: "VKAUTH", Polarity: "-", Scenario: "GET /kiosk-sut-v2/nonexistent — 404"},
			{Code: "TC-RB-060", EpicCode: "RB", Polarity: "+", SpecPaths: []string{"tests/web/sut-frontend/report-builder/list.spec.ts"}},
			{Code: "TC-AUTH-011", EpicCode: "AUTH", Polarity: "+", SpecPaths: []string{"tests/web/sut-frontend/auth/login.spec.ts"}},
			{Code: "TC-AUTH-020", EpicCode: "AUTH", Polarity: "+", SpecPaths: []string{"tests/web/sut-frontend/auth/login.spec.ts"}},
			{Code: "TC-PATREG-001", EpicCode: "AUTH", StoryCode: "US-PATREG-01", Polarity: "+"},
		},
		Files: files,
	}
}

type flatRoute struct{ surface, app, path, methods, sources, pages string }

func flat(inv Inventory) map[string]flatRoute {
	out := map[string]flatRoute{}
	for _, r := range inv.Routes {
		out[r.Surface+" "+r.App+" "+r.Path] = flatRoute{r.Surface, r.App, r.Path, strings.Join(r.Methods, ","), strings.Join(r.Sources, ","), strings.Join(r.PageObjects, ",")}
	}
	return out
}

func caseLinks(inv Inventory, code string) []string {
	path := map[string]string{}
	for _, r := range inv.Routes {
		path[r.Key] = r.Path
	}
	var out []string
	for _, l := range inv.Links {
		if l.RefType == RefCase && l.Ref == code {
			out = append(out, l.Source+":"+strings.TrimSpace(l.Method+" "+path[l.RouteKey]))
		}
	}
	return out
}

func TestBuild(t *testing.T) {
	inv := Build(testInput())
	routes := flat(inv)
	want := map[string]flatRoute{
		"api sut-vendor-backend /api/v1/service-status":        {methods: "DELETE,GET,POST", sources: "catalog,repo,scenario"},
		"api sut-vendor-backend /api/v1/swagger/*":             {sources: "catalog"},
		"api sut-vendor-backend /api/v1/swagger/:relativePath": {methods: "GET", sources: "repo"},
		"api sut-vendor-backend /api/v1/kiosk-sut-v2":          {sources: "catalog"},
		"api sut-vendor-backend /api/v1/kiosk-sut-v2/health":   {methods: "GET", sources: "scenario"},
		"api sut-vendor-backend /api/v1/kiosk-sut-v2/patients": {methods: "POST", sources: "repo"},
		"api sut-vendor-backend /api/v1/omnichannel/booking":   {methods: "POST", sources: "repo"},
		"api sut-vendor-backend /api/v1/vtt/patients":          {methods: "GET", sources: "repo"},
		"web sut-frontend /auth/login":                         {sources: "repo", pages: "LoginPage"},
		"web sut-frontend /auth/impersonated-login":            {sources: "repo", pages: "ImpersonatedLoginPage"},
		"web sut-frontend /report-builder/list":                {sources: "repo", pages: "ReportBuilderListPage"},
		"web sut-frontend /chart-management/db/new":            {sources: "repo", pages: "DashboardBuilderPage"},
		"web sut-frontend /chart-management/db/:param/edit":    {sources: "repo"},
		"web sut-frontend /master-patient":                     {sources: "catalog"},
	}
	for k, w := range want {
		got, ok := routes[k]
		if !ok {
			t.Errorf("missing route %s", k)
			continue
		}
		if got.methods != w.methods || got.sources != w.sources || got.pages != w.pages {
			t.Errorf("%s: methods %q sources %q pages %q, want %q %q %q", k, got.methods, got.sources, got.pages, w.methods, w.sources, w.pages)
		}
	}
	for _, gone := range []string{
		"api sut-vendor-backend /api/v1/thisisnotaroute",          // negative probe
		"api sut-vendor-backend /api/v1/kiosk-sut-v2/nonexistent", // negative probe
		"api sut-vendor-backend /api/v1/health",                   // a secondary mention
		"api sut-vendor-backend /api/v1/omnichannel/:operation",   // JSDoc placeholder of the class
		"api sut-vendor-backend /api/v1/omnichannel/:param",       // the private transport
		"web shared /never", "web sut-frontend /never",
	} {
		if _, ok := routes[gone]; ok {
			t.Errorf("unexpected route %s", gone)
		}
	}
	if len(routes) != len(want) {
		for k := range routes {
			if _, ok := want[k]; !ok {
				t.Errorf("extra route %s", k)
			}
		}
	}

	check := func(code string, want ...string) {
		t.Helper()
		got := caseLinks(inv, code)
		if !reflect.DeepEqual(sortCopy(got), sortCopy(want)) {
			t.Errorf("%s links = %v, want %v", code, got, want)
		}
	}
	check("TC-VSYS-001", "scenario:GET /api/v1/service-status", "spec:/api/v1/service-status", "feature:/api/v1/service-status")
	check("TC-VSYS-003", "scenario:POST /api/v1/service-status")
	check("TC-VSYS-006")
	check("TC-VKAUTH-007", "scenario:GET /api/v1/kiosk-sut-v2/health")
	check("TC-RB-060", "spec:/report-builder/list")
	// TC-AUTH-020 is named by a block; TC-AUTH-011 is not, so it gets every route of the spec.
	check("TC-AUTH-020", "spec:/auth/impersonated-login")
	check("TC-AUTH-011", "spec:/auth/impersonated-login", "spec:/auth/login")
	check("TC-PATREG-001", "feature:/master-patient")
}

func sortCopy(xs []string) []string {
	out := append([]string{}, xs...)
	for i := range out {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

func TestBuildDeterministic(t *testing.T) {
	in := testInput()
	a := Build(in)
	r := rand.New(rand.NewSource(1))
	for i := 0; i < 5; i++ {
		in2 := in
		in2.Cases = append([]Case{}, in.Cases...)
		r.Shuffle(len(in2.Cases), func(x, y int) { in2.Cases[x], in2.Cases[y] = in2.Cases[y], in2.Cases[x] })
		in2.Features = append([]Feature{}, in.Features...)
		r.Shuffle(len(in2.Features), func(x, y int) { in2.Features[x], in2.Features[y] = in2.Features[y], in2.Features[x] })
		if b := Build(in2); !reflect.DeepEqual(a, b) {
			t.Fatal("Build is not deterministic under input order")
		}
	}
}

func TestMountTie(t *testing.T) {
	in := Input{Features: []Feature{
		{Code: "F1", RepoEntry: "svc `/api/v2/a`"},
		{Code: "F2", RepoEntry: "svc `/api/v1/b`"},
		{Code: "F3", RepoEntry: "svc `/c`"},
	}}
	if _, ok := flat(Build(in))["api svc /api/v1/c"]; !ok {
		t.Errorf("tie should pick /api/v1: %v", flat(Build(in)))
	}
}

func TestOversizedFile(t *testing.T) {
	in := Input{Files: map[string][]byte{"src/web/app/x.page.ts": []byte("class X {\n  readonly path = '/x';\n" + strings.Repeat(" ", MaxFileBytes) + "}")}}
	inv := Build(in)
	if len(inv.Routes) != 0 || len(inv.Problems) != 1 {
		t.Fatalf("routes %v problems %v", inv.Routes, inv.Problems)
	}
}

func TestCoverageAndNext(t *testing.T) {
	inv := Build(testInput())
	links := map[string][]CaseLink{}
	byPath := map[string]Route{}
	for _, r := range inv.Routes {
		byPath[r.Path] = r
	}
	for _, l := range inv.Links {
		if l.RefType == RefCase && Direct(l.Source) {
			links[l.RouteKey] = append(links[l.RouteKey], CaseLink{Case: l.Ref, Method: l.Method, Source: l.Source})
		}
	}
	// A planned GET case and a planned POST case on the probe; the swagger scenario under the wildcard.
	ss := byPath["/api/v1/service-status"].Key
	links[ss] = append(links[ss], CaseLink{Case: "TC-VSYS-009", Method: "GET", Source: SourceScenario}, CaseLink{Case: "TC-VSYS-010", Method: "PUT", Source: SourceScenario})
	sw := byPath["/api/v1/swagger/:relativePath"].Key
	links[sw] = append(links[sw], CaseLink{Case: "TC-VSYS-004", Method: "GET", Source: SourceScenario})
	facts := map[string]CaseFact{
		"TC-VSYS-001":   {Code: "TC-VSYS-001", Status: "automated", Automated: true, Result: "pass", SpecPaths: []string{"tests/api/rest/sut-vendor-backend/TC-VSYS-001.spec.ts"}},
		"TC-VSYS-003":   {Code: "TC-VSYS-003", Status: "automated", Automated: true, Result: "fail"},
		"TC-VSYS-004":   {Code: "TC-VSYS-004", Status: "automated", Automated: true, Result: "pass"},
		"TC-VSYS-009":   {Code: "TC-VSYS-009", Status: "planned", Priority: "P2"},
		"TC-VSYS-010":   {Code: "TC-VSYS-010", Status: "planned", Priority: "P0"},
		"TC-VKAUTH-007": {Code: "TC-VKAUTH-007", Status: "planned", Priority: "P1"},
		"TC-RB-060":     {Code: "TC-RB-060", Status: "automated", Automated: true},
	}
	cov := Coverage(inv.Routes, links, facts)
	want := map[string]string{
		"/api/v1/service-status":      StatusFailing,
		"/api/v1/swagger/*":           StatusPassing, // rolled up from /api/v1/swagger/:relativePath
		"/api/v1/kiosk-sut-v2/health": StatusPlanned,
		"/api/v1/kiosk-sut-v2":        StatusPlanned, // a catalog prefix with no case of its own
		"/report-builder/list":        StatusAutomated,
		"/api/v1/omnichannel/booking": StatusUntested,
	}
	for p, w := range want {
		if got := cov[byPath[p].Key].Status; got != w {
			t.Errorf("%s status %s, want %s", p, got, w)
		}
	}
	eligible := map[string]bool{"TC-VSYS-009": true, "TC-VSYS-010": true, "TC-VKAUTH-007": true}
	next := Next(inv.Routes, links, facts, eligible, 25)
	if len(next) != 2 {
		t.Fatalf("next = %+v", next)
	}
	if next[0].CaseCode != "TC-VSYS-009" || next[0].Tier != 1 || next[0].ExampleCase != "TC-VSYS-001" || next[0].ExampleSpec == "" {
		t.Errorf("first = %+v", next[0])
	}
	if next[1].CaseCode != "TC-VSYS-010" || next[1].Tier != 2 {
		t.Errorf("second = %+v", next[1])
	}
}
