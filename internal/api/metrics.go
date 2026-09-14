package api

import (
	"fmt"
	"io"
	"strconv"
	"strings"
)

// MetricsSnapshot contains only bounded-cardinality operational measurements.
// Identifiers, content, filesystem paths, and secrets are deliberately absent.
type MetricsSnapshot struct {
	Network                 string
	ProtocolVersion         string
	SoftwareVersion         string
	Ready                   bool
	SyncStatus              string
	UptimeSeconds           float64
	ConnectedPeers          int
	OutboundPeers           int
	MaxConnectedPeers       int
	ConsensusHeight         int64
	ConsensusActive         bool
	ValidatorAuthorized     bool
	StateHeight             int64
	Identities              int
	GovernanceProposals     int
	ResearchEntries         int
	ResourceEntries         int
	BoardPosts              int
	BoardReplies            int
	ObjectCount             uint64
	ObjectBytes             uint64
	ObjectQuotaBytes        uint64
	RecentTransactions      int
	RecentTransactionLimit  int
	SyncSnapshotLimitBytes  int
	ObjectFetchSuccessTotal uint64
	ObjectFetchFailureTotal uint64
}

type metricsProvider interface{ Metrics() MetricsSnapshot }

func writeMetrics(writer io.Writer, m MetricsSnapshot, requests, rejections uint64) {
	label := fmt.Sprintf("network=%s,protocol=%s,software=%s", quoteLabel(m.Network), quoteLabel(m.ProtocolVersion), quoteLabel(m.SoftwareVersion))
	metric(writer, "zion_node_info", "ZION software and protocol build information.", 1, label)
	metric(writer, "zion_node_ready", "Whether the local runtime is ready.", boolNumber(m.Ready), "")
	metric(writer, "zion_node_uptime_seconds", "Process-local runtime uptime.", m.UptimeSeconds, "")
	metric(writer, "zion_node_sync_status", "Current bounded synchronization state.", 1, "status="+quoteLabel(m.SyncStatus))
	metric(writer, "zion_p2p_connected_peers", "Usable authenticated ZION peers.", m.ConnectedPeers, "")
	metric(writer, "zion_p2p_outbound_peers", "Usable peers with an outbound connection.", m.OutboundPeers, "")
	metric(writer, "zion_p2p_max_connected_peers", "Configured P2P peer limit.", m.MaxConnectedPeers, "")
	metric(writer, "zion_consensus_height", "Latest accepted consensus height.", m.ConsensusHeight, "")
	metric(writer, "zion_consensus_active", "Whether the consensus service is active.", boolNumber(m.ConsensusActive), "")
	metric(writer, "zion_validator_authorized", "Whether the configured validator is canonically authorized.", boolNumber(m.ValidatorAuthorized), "")
	metric(writer, "zion_state_height", "Durable canonical state height.", m.StateHeight, "")
	metric(writer, "zion_state_identities", "Canonical identity count.", m.Identities, "")
	metric(writer, "zion_governance_proposals", "Canonical governance proposal count.", m.GovernanceProposals, "")
	metric(writer, "zion_research_entries", "Canonical Research entry count.", m.ResearchEntries, "")
	metric(writer, "zion_resource_entries", "Canonical Resource entry count.", m.ResourceEntries, "")
	metric(writer, "zion_board_posts", "Locally indexed Board post count.", m.BoardPosts, "")
	metric(writer, "zion_board_replies", "Locally indexed Board reply count.", m.BoardReplies, "")
	metric(writer, "zion_object_store_objects", "Local object count.", m.ObjectCount, "")
	metric(writer, "zion_object_store_bytes", "Local object bytes.", m.ObjectBytes, "")
	metric(writer, "zion_object_store_quota_bytes", "Configured local object quota.", m.ObjectQuotaBytes, "")
	metric(writer, "zion_recent_transactions", "Bounded recent transaction cache entries.", m.RecentTransactions, "")
	metric(writer, "zion_recent_transactions_limit", "Recent transaction cache limit.", m.RecentTransactionLimit, "")
	metric(writer, "zion_state_sync_snapshot_limit_bytes", "Maximum accepted state-sync snapshot size.", m.SyncSnapshotLimitBytes, "")
	metric(writer, "zion_object_fetch_success_total", "Successful network object fetches.", m.ObjectFetchSuccessTotal, "")
	metric(writer, "zion_object_fetch_failure_total", "Failed network object fetches.", m.ObjectFetchFailureTotal, "")
	metric(writer, "zion_api_requests_total", "Local API requests handled.", requests, "")
	metric(writer, "zion_api_rejections_total", "Local API requests rejected.", rejections, "")
}

func metric(writer io.Writer, name, help string, value any, labels string) {
	kind := "gauge"
	if strings.HasSuffix(name, "_total") {
		kind = "counter"
	}
	fmt.Fprintf(writer, "# HELP %s %s\n# TYPE %s %s\n", name, help, name, kind)
	if labels != "" {
		fmt.Fprintf(writer, "%s{%s} %v\n", name, labels, value)
		return
	}
	fmt.Fprintf(writer, "%s %v\n", name, value)
}

func quoteLabel(value string) string { return strconv.Quote(value) }
func boolNumber(value bool) int {
	if value {
		return 1
	}
	return 0
}
