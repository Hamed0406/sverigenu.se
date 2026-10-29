package fetcher

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const braBase = "https://statistik.bra.se/solwebb/action"

// statistik.bra.se serves its leaf certificate with a mismatched intermediate
// (GeoTrust EV RSA CA 2018 instead of the issuing DigiCert EV RSA CA G2), so
// chain building fails against the system roots alone. Embedding the correct
// intermediate (a public DigiCert CA, valid until 2030-07-02) lets us verify
// the chain without disabling TLS verification.
const digiCertEVRSACAG2 = `-----BEGIN CERTIFICATE-----
MIIFPDCCBCSgAwIBAgIQAWePH++IIlXYsKcOa3uyIDANBgkqhkiG9w0BAQsFADBh
MQswCQYDVQQGEwJVUzEVMBMGA1UEChMMRGlnaUNlcnQgSW5jMRkwFwYDVQQLExB3
d3cuZGlnaWNlcnQuY29tMSAwHgYDVQQDExdEaWdpQ2VydCBHbG9iYWwgUm9vdCBH
MjAeFw0yMDA3MDIxMjQyNTBaFw0zMDA3MDIxMjQyNTBaMEQxCzAJBgNVBAYTAlVT
MRUwEwYDVQQKEwxEaWdpQ2VydCBJbmMxHjAcBgNVBAMTFURpZ2lDZXJ0IEVWIFJT
QSBDQSBHMjCCASIwDQYJKoZIhvcNAQEBBQADggEPADCCAQoCggEBAK0eZsx/neTr
f4MXJz0R2fJTIDfN8AwUAu7hy4gI0vp7O8LAAHx2h3bbf8wl+pGMSxaJK9ffDDCD
63FqqFBqE9eTmo3RkgQhlu55a04LsXRLcK6crkBOO0djdonybmhrfGrtBqYvbRat
xenkv0Sg4frhRl4wYh4dnW0LOVRGhbt1G5Q19zm9CqMlq7LlUdAE+6d3a5++ppfG
cnWLmbEVEcLHPAnbl+/iKauQpQlU1Mi+wEBnjE5tK8Q778naXnF+DsedQJ7NEi+b
QoonTHEz9ryeEcUHuQTv7nApa/zCqes5lXn1pMs4LZJ3SVgbkTLj+RbBov/uiwTX
tkBEWawvZH8CAwEAAaOCAgswggIHMB0GA1UdDgQWBBRqTlC/mGidW3sgddRZAXlI
ZpIyBjAfBgNVHSMEGDAWgBROIlQgGJXm427mD/r6uRLtBhePOTAOBgNVHQ8BAf8E
BAMCAYYwHQYDVR0lBBYwFAYIKwYBBQUHAwEGCCsGAQUFBwMCMBIGA1UdEwEB/wQI
MAYBAf8CAQAwNAYIKwYBBQUHAQEEKDAmMCQGCCsGAQUFBzABhhhodHRwOi8vb2Nz
cC5kaWdpY2VydC5jb20wewYDVR0fBHQwcjA3oDWgM4YxaHR0cDovL2NybDMuZGln
aWNlcnQuY29tL0RpZ2lDZXJ0R2xvYmFsUm9vdEcyLmNybDA3oDWgM4YxaHR0cDov
L2NybDQuZGlnaWNlcnQuY29tL0RpZ2lDZXJ0R2xvYmFsUm9vdEcyLmNybDCBzgYD
VR0gBIHGMIHDMIHABgRVHSAAMIG3MCgGCCsGAQUFBwIBFhxodHRwczovL3d3dy5k
aWdpY2VydC5jb20vQ1BTMIGKBggrBgEFBQcCAjB+DHxBbnkgdXNlIG9mIHRoaXMg
Q2VydGlmaWNhdGUgY29uc3RpdHV0ZXMgYWNjZXB0YW5jZSBvZiB0aGUgUmVseWlu
ZyBQYXJ0eSBBZ3JlZW1lbnQgbG9jYXRlZCBhdCBodHRwczovL3d3dy5kaWdpY2Vy
dC5jb20vcnBhLXVhMA0GCSqGSIb3DQEBCwUAA4IBAQBSMgrCdY2+O9spnYNvwHiG
+9lCJbyELR0UsoLwpzGpSdkHD7pVDDFJm3//B8Es+17T1o5Hat+HRDsvRr7d3MEy
o9iXkkxLhKEgApA2Ft2eZfPrTolc95PwSWnn3FZ8BhdGO4brTA4+zkPSKoMXi/X+
WLBNN29Z/nbCS7H/qLGt7gViEvTIdU8x+H4l/XigZMUDaVmJ+B5d7cwSK7yOoQdf
oIBGmA5Mp4LhMzo52rf//kXPfE3wYIZVHqVuxxlnTkFYmffCX9/Lon7SWaGdg6Rc
k4RHhHLWtmz2lTZ5CEo2ljDsGzCFGJP7oT4q6Q8oFC38irvdKIJ95cUxYzj4tnOI
-----END CERTIFICATE-----`

// braCategories are the reported-crime categories we collect from Sol.
// IDs are Sol brottstyp IDs from the menyid=101 selection page (arrayNivaett).
var braCategories = []struct {
	id   int
	slug string
	name string
}{
	{5036, "totalt", "Totalt antal brott"},
	{11130, "misshandel", "Misshandel inkl. grov"},
	{11338, "valdtakt", "Våldtäkt inkl. grov"},
	{11091, "frihet_frid", "Brott mot frihet och frid (4 kap.)"},
	{11462, "inbrott_bostad", "Inbrottsstöld i bostad"},
	{11522, "ran", "Rån inkl. grovt"},
	{11447, "biltillgrepp", "Tillgrepp av motorfordon"},
	{11099, "skadegorelse", "Skadegörelsebrott (12 kap.)"},
	{11650, "narkotika", "Brott mot narkotikastrafflagen"},
	{11096, "bedrageri", "Bedrägeri och annan oredlighet (9 kap.)"},
}

// BRACrime is one (year, category, kommun) cell from the Sol database.
// Count or Per100k can be nil when the source table has no value.
type BRACrime struct {
	Year       int
	CategoryID int
	Slug       string
	Category   string
	KommunID   int
	Kommun     string
	CountyCode string
	Count      *int
	Per100k    *float64
}

// arrayPeriod entries look like: arrayPeriod[1]="2821*2024*311*År, 2024*-1"
// — capture the Sol period ID and the year it represents.
var braPeriodRe = regexp.MustCompile(`arrayPeriod\[\d+\]="(\d+)\*(\d{4})\*311\*`)

// Result cells carry composite headers: period(311=årsvis) brottstyp region mått.
// e.g. <td align="right" class="resultatAntal" headers="2821 311 5036 8617 antal">2&nbsp;537</td>
var braCellRe = regexp.MustCompile(`headers="(\d+) 311 (\d+) (\d+) (antal|antal_100)"[^>]*>([^<]*)`)

// newBRAClient builds an HTTP client for statistik.bra.se: a cookie jar for
// the JSESSIONID the Sol app requires, and a root pool extended with the
// DigiCert intermediate (see digiCertEVRSACAG2 above).
func newBRAClient() (*http.Client, error) {
	roots, err := x509.SystemCertPool()
	if err != nil {
		roots = x509.NewCertPool()
	}
	if !roots.AppendCertsFromPEM([]byte(digiCertEVRSACAG2)) {
		return nil, fmt.Errorf("bra: failed to parse embedded DigiCert intermediate")
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	return &http.Client{
		Timeout: 60 * time.Second,
		Jar:     jar,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: roots},
		},
	}, nil
}

// FetchBRACrimes scrapes reported crimes per kommun, category and year from
// BRÅ's Sol statistics database (statistik.bra.se). Sol has no JSON API or
// file export, but the result table cells carry machine-readable composite
// headers attributes, and the search flow is three plain HTTP requests:
//
//	POST anmalda/urval/vantapopup  (selection IDs, stored in the session)
//	GET  anmalda/urval/sok         (executes the search)
//	GET  anmalda/urval/soktabell   (renders the result table)
//
// One search per year covers all 291 kommuner × all categories × both
// measures (~5,800 cells), staying under Sol's 10,000-cell limit. Available
// years (1996–) are discovered from the selection page so new years are
// picked up automatically. Years that fail are skipped with a warning; if
// every year fails an error is returned so the sync log records the outage.
func (c *Client) FetchBRACrimes(ctx context.Context) ([]BRACrime, error) {
	httpc, err := newBRAClient()
	if err != nil {
		return nil, err
	}

	// Step 1: prime the session. The selection page returns an error stub
	// unless the session has both a JSESSIONID (set by the start page) and
	// the anmälda menu catalog opened (start?menykatalogid=1).
	if _, err := braGet(ctx, httpc, c.braURL+"/start"); err != nil {
		return nil, fmt.Errorf("bra: start page: %w", err)
	}
	if _, err := braGet(ctx, httpc, c.braURL+"/start?menykatalogid=1"); err != nil {
		return nil, fmt.Errorf("bra: menu catalog: %w", err)
	}

	// Step 2: load the selection page, which lists the available periods.
	body, err := braGet(ctx, httpc, c.braURL+"/anmalda/urval/urval?menyid=101")
	if err != nil {
		return nil, fmt.Errorf("bra: selection page: %w", err)
	}
	periods := map[int]string{} // year -> Sol period ID
	for _, m := range braPeriodRe.FindAllStringSubmatch(body, -1) {
		if y, err := strconv.Atoi(m[2]); err == nil {
			periods[y] = m[1]
		}
	}
	if len(periods) == 0 {
		return nil, fmt.Errorf("bra: no periods found on selection page (layout changed?)")
	}
	years := make([]int, 0, len(periods))
	for y := range periods {
		years = append(years, y)
	}
	sort.Ints(years)

	catIDs := make([]string, len(braCategories))
	catBy := map[int]int{} // category ID -> index in braCategories
	for i, c := range braCategories {
		catIDs[i] = strconv.Itoa(c.id)
		catBy[c.id] = i
	}
	regIDs := make([]string, 0, len(braKommun))
	for id := range braKommun {
		regIDs = append(regIDs, strconv.Itoa(id))
	}
	sort.Strings(regIDs)

	var out []BRACrime
	failed := 0
	for _, year := range years {
		rows, err := c.fetchBRAYear(ctx, httpc, periods[year], catIDs, regIDs, catBy, year)
		if err != nil {
			slog.Warn("bra year fetch failed", "year", year, "err", err)
			failed++
			continue
		}
		out = append(out, rows...)

		// Be polite to the shared Sol backend between searches.
		select {
		case <-time.After(300 * time.Millisecond):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if failed == len(years) {
		return nil, fmt.Errorf("bra: all %d years failed to fetch", failed)
	}
	return out, nil
}

// fetchBRAYear runs one Sol search (all kommuner × all categories for a
// single year) and parses the result table into rows.
func (c *Client) fetchBRAYear(ctx context.Context, httpc *http.Client, periodID string, catIDs, regIDs []string, catBy map[int]int, year int) ([]BRACrime, error) {
	form := url.Values{
		"brottstyp_id_string": {strings.Join(catIDs, "*")},
		"region_id_string":    {strings.Join(regIDs, "*")},
		"period_id_string":    {periodID},
		"antal":               {"1"},
		"antal_100k":          {"1"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.braURL+"/anmalda/urval/vantapopup", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := httpc.Do(req)
	if err != nil {
		return nil, err
	}
	io.Copy(io.Discard, resp.Body) //nolint:errcheck
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("vantapopup returned %d", resp.StatusCode)
	}

	// The wait popup meta-refreshes to sok, which executes the search.
	if _, err := braGet(ctx, httpc, c.braURL+"/anmalda/urval/sok"); err != nil {
		return nil, fmt.Errorf("sok: %w", err)
	}
	table, err := braGet(ctx, httpc, c.braURL+"/anmalda/urval/soktabell")
	if err != nil {
		return nil, fmt.Errorf("soktabell: %w", err)
	}

	// Merge the antal and antal_100 cells of each (category, kommun) pair.
	type key struct{ cat, kommun int }
	merged := map[key]*BRACrime{}
	var order []key
	for _, m := range braCellRe.FindAllStringSubmatch(table, -1) {
		catID, _ := strconv.Atoi(m[2])
		regID, _ := strconv.Atoi(m[3])
		ci, ok := catBy[catID]
		if !ok {
			continue
		}
		info, ok := braKommun[regID]
		if !ok {
			continue
		}
		k := key{catID, regID}
		row := merged[k]
		if row == nil {
			row = &BRACrime{
				Year:       year,
				CategoryID: catID,
				Slug:       braCategories[ci].slug,
				Category:   braCategories[ci].name,
				KommunID:   regID,
				Kommun:     info.Name,
				CountyCode: info.County,
			}
			merged[k] = row
			order = append(order, k)
		}
		if v, ok := braParseNumber(m[5]); ok {
			if m[4] == "antal" {
				n := int(v)
				row.Count = &n
			} else {
				f := v
				row.Per100k = &f
			}
		}
	}
	if len(merged) == 0 {
		return nil, fmt.Errorf("no data cells in result table")
	}
	rows := make([]BRACrime, 0, len(order))
	for _, k := range order {
		rows = append(rows, *merged[k])
	}
	return rows, nil
}

// braParseNumber parses a Sol table cell value like "2&nbsp;537" or "7 810".
// Missing values render as ".." or empty — those return ok=false.
func braParseNumber(s string) (float64, bool) {
	s = strings.ReplaceAll(s, "&nbsp;", "")
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "\xa0", "")
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", ".")
	if s == "" || strings.Contains(s, ".." ) {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// braGet fetches a Sol page and returns the body as a string. Sol serves
// ISO-8859-1, but everything we parse (IDs, digits, attribute names) is
// ASCII so no transcoding is needed.
func braGet(ctx context.Context, httpc *http.Client, rawURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := httpc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body) //nolint:errcheck
		return "", fmt.Errorf("upstream returned %d for %s", resp.StatusCode, rawURL)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	return string(b), err
}
