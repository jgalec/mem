package store

var validNodeTypes = map[string]bool{
	"Project": true, "Feature": true, "Session": true, "Decision": true,
	"Evidence": true, "Lesson": true, "Blocker": true, "File": true, "Command": true,
}

var validRelationships = map[string]bool{
	"WORKED_ON": true, "TOUCHED": true, "MADE": true, "SUPPORTED_BY": true,
	"PRODUCED": true, "FROM_COMMAND": true, "DERIVED_FROM": true,
	"BLOCKED_BY": true, "DEPENDS_ON": true,
}

func graphTypeList() string {
	return "Project, Feature, Session, Decision, Evidence, Lesson, Blocker, File, Command"
}

func graphRelList() string {
	return "WORKED_ON, TOUCHED, MADE, SUPPORTED_BY, PRODUCED, FROM_COMMAND, DERIVED_FROM, BLOCKED_BY, DEPENDS_ON"
}
