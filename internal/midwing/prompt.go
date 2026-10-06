package midwing

import _ "embed"

//go:embed skill/SKILL.md
var agentSkill string

//go:embed skill/install-prompt.md
var installRequest string

// AgentPrompt is service-neutral and does not inspect accounts or credentials.
func (s *Store) AgentPrompt() (string, error) {
	return installRequest + "\n```markdown\n" + agentSkill + "\n```\n", nil
}
