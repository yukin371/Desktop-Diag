//go:build windows

// Summarizes diagnostic domains and connects overview findings to local details.
package report

import "github.com/yukin371/desktop-diag/internal/model"

// htmlOverview is one domain summary; missing data never produces a normal status.
type htmlOverview struct {
	Title, Target, Status, Class string
	Severe, Warning, Missing     int
	Links                        []htmlOverviewLink
	observed                     bool
}

// htmlOverviewLink connects one issue title to its unique report anchor.
type htmlOverviewLink struct{ ID, Label string }

// buildHTMLOverview keeps each finding in one domain and completeness counts explicitly aggregate.
func buildHTMLOverview(snap *model.Snapshot, issues []model.Issue, findings []htmlFinding, sections []htmlSection) []htmlOverview {
	rows := []htmlOverview{
		{Title: layer2SubHost, observed: snap.Host.ComputerName != "" && snap.Host.OSName != ""},
		{Title: layer2SubAdapters, observed: len(snap.Adapters) > 0},
		{Title: layer2SubHealth, observed: snap.Health.CPUKnown && snap.Health.MemKnown && snap.Health.DiskKnown},
		{Title: layer2SubConnect, observed: htmlProbesComplete(snap.Probes)},
		{Title: "诊断完整性", Target: "findings", observed: true, Missing: len(snap.Failures)},
	}
	for index := range rows {
		row := &rows[index]
		if row.Target == "" {
			row.Target = "findings"
		}
		for _, section := range sections {
			if section.Title == row.Title {
				row.Target = section.ID
			}
		}
		for _, failure := range snap.Failures {
			if htmlFailureDomain(failure.EnvVar) == row.Title {
				row.Missing++
			}
		}
		for issueIndex, issue := range issues {
			if htmlIssueDomain(issue) != row.Title {
				continue
			}
			switch issue.Severity {
			case model.SevSevere:
				row.Severe++
			case model.SevWarning:
				row.Warning++
			}
			row.Links = append(row.Links, htmlOverviewLink{findings[issueIndex].ID, issue.Title})
		}
		row.Status, row.Class = "未发现告警", "normal"
		switch {
		case row.Severe > 0:
			row.Status, row.Class = "存在严重告警", "severe"
		case row.Missing > 0:
			row.Status, row.Class = "数据不完整", "warning"
		case row.Warning > 0:
			row.Status, row.Class = "存在警告", "warning"
		case !row.observed:
			row.Status, row.Class = "未取得完整数据", "warning"
		}
		if row.Title == "诊断完整性" && row.Missing > 0 {
			row.Target = "completeness"
		}
	}
	return rows
}

// htmlIssueDomain maps fixed rule identities; future rules fall back to their declared category.
func htmlIssueDomain(issue model.Issue) string {
	switch issue.RuleID {
	case "R-01", "R-02", "R-03", "R-04", "R-05":
		return layer2SubAdapters
	case "R-06", "R-07", "R-08", "R-09", "R-10", "R-11":
		return layer2SubHealth
	case "R-12", "R-13", "R-14", "R-15", "R-16", "R-17", "R-18":
		return layer2SubConnect
	case "R-19":
		return "诊断完整性"
	}
	switch issue.Category {
	case model.CatNetwork:
		return layer2SubConnect
	case model.CatSystem, model.CatStorage:
		return layer2SubHealth
	default:
		return "诊断完整性"
	}
}

// htmlFailureDomain uses collector identity rather than parsing translated reason strings.
func htmlFailureDomain(env string) string {
	switch env {
	case "system":
		return layer2SubHost
	case "network":
		return layer2SubAdapters
	case "health":
		return layer2SubHealth
	case "probe":
		return layer2SubConnect
	default:
		return ""
	}
}

// htmlProbesComplete requires all four probe kinds and rejects skipped or partial records.
func htmlProbesComplete(probes []model.ProbeResult) bool {
	kinds := map[model.ProbeKind]bool{}
	for _, probe := range probes {
		if !probe.Executed() || probe.Incomplete || probe.Err != "" {
			return false
		}
		kinds[probe.Kind] = true
	}
	return kinds[model.ProbeICMPGateway] && kinds[model.ProbeDNSSystem] && kinds[model.ProbeDNSDirect] && kinds[model.ProbeTCP443]
}
