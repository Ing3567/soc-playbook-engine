package main

import (
	"fmt"
	"strings"
	"time"
)

// PreparePlaybookVersion assigns an auto-incrementing version number and constructs
// an immutable PlaybookVersionSnapshot for archival in the playbook_versions collection.
func PreparePlaybookVersion(doc PlaybookDocument, prevVersion int, now time.Time) (PlaybookDocument, PlaybookVersionSnapshot) {
	if strings.TrimSpace(doc.Title) == "" {
		doc.Title = "Untitled SOP"
	}
	if strings.TrimSpace(doc.ID) == "" {
		doc.ID = fmt.Sprintf("sop_%d", now.UnixNano())
	}
	if prevVersion < 0 {
		prevVersion = 0
	}
	doc.Version = prevVersion + 1
	doc.UpdatedAt = now

	snapshot := PlaybookVersionSnapshot{
		ID:         fmt.Sprintf("%s_v%d", doc.ID, doc.Version),
		PlaybookID: doc.ID,
		Version:    doc.Version,
		Title:      doc.Title,
		RawText:    doc.RawText,
		Engine:     doc.Engine,
		Nodes:      doc.Nodes,
		Edges:      doc.Edges,
		CreatedAt:  now,
	}
	return doc, snapshot
}
