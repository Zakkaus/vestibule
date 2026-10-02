package lookup

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
)

// cveIDRe matches the published identifier form and nothing that could steer the request.
var CveIDRe = regexp.MustCompile(`^(?i)(CVE-[0-9]{4}-[0-9]{4,10})$`)

// cveBodyLimit bounds one record; the largest carry long descriptions and many references.
const cveBodyLimit = 1024 * 1024

// cveDescriptionLimit keeps a long advisory readable in a chat message.
const cveDescriptionLimit = 600

type CVERecord struct {
	Severity    string
	Score       string
	Published   string
	Status      string
	Description string
}

func FetchCVE(ctx context.Context, id string) (record CVERecord, found, failed bool) {
	body, err := httpGetBody(ctx, "https://services.nvd.nist.gov/rest/json/cves/2.0?cveId="+id, cveBodyLimit)
	if err != nil {
		var status *httpStatusError
		if errors.As(err, &status) && status.Code == 404 {
			return CVERecord{}, false, false
		}
		return CVERecord{}, false, true
	}
	var parsed struct {
		Vulnerabilities []struct {
			CVE struct {
				ID           string `json:"id"`
				Published    string `json:"published"`
				VulnStatus   string `json:"vulnStatus"`
				Descriptions []struct {
					Lang  string `json:"lang"`
					Value string `json:"value"`
				} `json:"descriptions"`
				Metrics struct {
					V31 []cveMetric `json:"cvssMetricV31"`
					V30 []cveMetric `json:"cvssMetricV30"`
					V2  []cveMetric `json:"cvssMetricV2"`
				} `json:"metrics"`
			} `json:"cve"`
		} `json:"vulnerabilities"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return CVERecord{}, false, true
	}
	if len(parsed.Vulnerabilities) == 0 {
		return CVERecord{}, false, false
	}
	entry := parsed.Vulnerabilities[0].CVE
	record.Published = strings.SplitN(entry.Published, "T", 2)[0]
	record.Status = entry.VulnStatus
	for _, d := range entry.Descriptions {
		if d.Lang == "en" {
			record.Description = capRunes(d.Value, cveDescriptionLimit)
			break
		}
	}
	// Prefer the newest scoring version the record carries; an older one is better than none.
	for _, metrics := range [][]cveMetric{entry.Metrics.V31, entry.Metrics.V30, entry.Metrics.V2} {
		if len(metrics) == 0 {
			continue
		}
		record.Severity = metrics[0].CVSSData.BaseSeverity
		record.Score = strconv.FormatFloat(metrics[0].CVSSData.BaseScore, 'f', -1, 64)
		break
	}
	return record, true, false
}

type cveMetric struct {
	CVSSData struct {
		BaseScore    float64 `json:"baseScore"`
		BaseSeverity string  `json:"baseSeverity"`
	} `json:"cvssData"`
}

// capRunes truncates on a rune boundary so a cut never splits a character.
func capRunes(s string, limit int) string {
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return strings.TrimSpace(string(runes[:limit])) + "…"
}
