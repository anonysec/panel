package health

// RuleBasedAnalyzer implements the Analyzer interface using pattern matching
// and threshold-based heuristics. It inspects the CheckResults and EventHistory
// to determine a likely root cause and suggest actions.
type RuleBasedAnalyzer struct{}

// Analyze performs rule-based root cause analysis on the given health data.
// It pattern-matches combinations of check results to identify systemic issues.
func (rba *RuleBasedAnalyzer) Analyze(input AnalysisInput) (AnalysisOutput, error) {
	if len(input.CheckResults) == 0 {
		return AnalysisOutput{
			RootCause:          "No issues detected",
			Confidence:         1.0,
			SuggestedActions:   []string{"System is operating normally"},
			AffectedComponents: []string{},
		}, nil
	}

	// Collect non-healthy checks
	var issues []CheckResult
	for _, cr := range input.CheckResults {
		if cr.Severity != SeverityHealthy {
			issues = append(issues, cr)
		}
	}

	if len(issues) == 0 {
		return AnalysisOutput{
			RootCause:          "All checks healthy",
			Confidence:         1.0,
			SuggestedActions:   []string{"No action required"},
			AffectedComponents: []string{},
		}, nil
	}

	// Classify issues by category/name for pattern matching
	nodeOffline := false
	vpnDown := false
	highDisk := false
	highMemory := false
	highCPU := false
	staleSessions := false
	expiredSubs := false
	dnsFailover := false
	dbDown := false

	var criticalCount int
	var affectedComponents []string

	for _, issue := range issues {
		affectedComponents = append(affectedComponents, issue.Name)
		if issue.Severity == SeverityCritical {
			criticalCount++
		}

		switch issue.Name {
		case "node_online_status":
			nodeOffline = true
		case "vpn_service_health":
			vpnDown = true
		case "disk_usage":
			highDisk = true
		case "memory_usage":
			highMemory = true
		case "cpu_usage":
			highCPU = true
		case "stale_sessions":
			staleSessions = true
		case "expired_subscriptions":
			expiredSubs = true
		case "dns_failover_status":
			dnsFailover = true
		case "database_connectivity":
			dbDown = true
		}
	}

	// Pattern matching for root cause analysis

	// Database down is the most critical — everything depends on it
	if dbDown {
		return AnalysisOutput{
			RootCause:          "Database connectivity failure",
			Confidence:         0.95,
			SuggestedActions:   []string{"Check database server status", "Verify database connection settings", "Check disk space on database server"},
			AffectedComponents: affectedComponents,
		}, nil
	}

	// Node offline + VPN down = Node failure
	if nodeOffline && vpnDown {
		actions := []string{"Check node connectivity and network path", "Review node logs for crash indicators", "Consider triggering DNS failover if node remains offline"}
		if dnsFailover {
			actions = append(actions, "DNS failover already in progress — monitor propagation")
		}
		return AnalysisOutput{
			RootCause:          "Node failure — node offline with VPN services down",
			Confidence:         0.9,
			SuggestedActions:   actions,
			AffectedComponents: affectedComponents,
		}, nil
	}

	// High disk + VPN crash = Disk causing service failure
	if highDisk && vpnDown {
		return AnalysisOutput{
			RootCause:          "Disk space causing service failure — high disk usage correlated with VPN crash",
			Confidence:         0.85,
			SuggestedActions:   []string{"Free disk space on affected nodes", "Clean up old logs and backups", "Restart VPN services after freeing space"},
			AffectedComponents: affectedComponents,
		}, nil
	}

	// High disk + high memory = Resource exhaustion
	if highDisk && highMemory {
		return AnalysisOutput{
			RootCause:          "Resource exhaustion — both disk and memory usage are critical",
			Confidence:         0.8,
			SuggestedActions:   []string{"Investigate high memory processes", "Clean up disk space", "Consider scaling infrastructure"},
			AffectedComponents: affectedComponents,
		}, nil
	}

	// Multiple critical issues = Multiple system degradation
	if criticalCount >= 3 {
		return AnalysisOutput{
			RootCause:          "Multiple system degradation — several critical issues detected simultaneously",
			Confidence:         0.7,
			SuggestedActions:   []string{"Investigate common root cause (network, hardware, or infrastructure issue)", "Check for recent changes or deployments", "Consider emergency maintenance window"},
			AffectedComponents: affectedComponents,
		}, nil
	}

	// Node offline + stale sessions = Node failure with user impact
	if nodeOffline && staleSessions {
		return AnalysisOutput{
			RootCause:          "Node failure with user impact — stale sessions indicate disconnected users",
			Confidence:         0.85,
			SuggestedActions:   []string{"Clear stale sessions", "Check node connectivity", "Notify affected users if extended outage"},
			AffectedComponents: affectedComponents,
		}, nil
	}

	// VPN down alone
	if vpnDown {
		return AnalysisOutput{
			RootCause:          "VPN service failure — one or more VPN services are not running",
			Confidence:         0.8,
			SuggestedActions:   []string{"Restart affected VPN services via node tasks", "Check service logs for crash reason"},
			AffectedComponents: affectedComponents,
		}, nil
	}

	// High CPU + high memory = resource pressure
	if highCPU && highMemory {
		return AnalysisOutput{
			RootCause:          "Resource pressure — high CPU and memory usage detected",
			Confidence:         0.75,
			SuggestedActions:   []string{"Identify resource-intensive processes", "Consider load balancing or horizontal scaling"},
			AffectedComponents: affectedComponents,
		}, nil
	}

	// Node offline alone
	if nodeOffline {
		return AnalysisOutput{
			RootCause:          "Node connectivity issue — one or more nodes not reporting",
			Confidence:         0.8,
			SuggestedActions:   []string{"Check node network connectivity", "Verify node agent is running"},
			AffectedComponents: affectedComponents,
		}, nil
	}

	// Expired subscriptions with stale sessions might indicate billing issues
	if expiredSubs && staleSessions {
		return AnalysisOutput{
			RootCause:          "Subscription management issue — expired accounts with lingering sessions",
			Confidence:         0.7,
			SuggestedActions:   []string{"Review expired accounts", "Clear stale sessions for expired users", "Check subscription renewal workflow"},
			AffectedComponents: affectedComponents,
		}, nil
	}

	// Default: just list the issues found
	var actionList []string
	for _, issue := range issues {
		if len(issue.SuggestedActions) > 0 {
			actionList = append(actionList, issue.SuggestedActions...)
		}
	}
	if len(actionList) == 0 {
		actionList = []string{"Review individual check results for details"}
	}

	// Deduplicate actions
	actionList = deduplicateStrings(actionList)

	return AnalysisOutput{
		RootCause:          summarizeIssues(issues),
		Confidence:         0.6,
		SuggestedActions:   actionList,
		AffectedComponents: affectedComponents,
	}, nil
}

// summarizeIssues creates a human-readable summary of the detected issues.
func summarizeIssues(issues []CheckResult) string {
	if len(issues) == 1 {
		return issues[0].Message
	}
	names := make([]string, 0, len(issues))
	for _, issue := range issues {
		names = append(names, issue.Name)
	}
	return "Multiple issues detected: " + joinStrings(names, ", ")
}

// joinStrings joins a slice of strings with a separator.
func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
}

// deduplicateStrings removes duplicate strings from a slice while preserving order.
func deduplicateStrings(input []string) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0, len(input))
	for _, s := range input {
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
			result = append(result, s)
		}
	}
	return result
}
