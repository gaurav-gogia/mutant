package sweep

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	yaml "go.yaml.in/yaml/v3"
)

// The topic playbooks are the case, cut down -- never a second story.
//
// examples/workshop/evidence/topics/ holds one playbook per DFRWS topic, so an
// instructor can build the timestomp image, the ad history and cookies, or the
// registry hive without the rest of the case, and without the generated PE that
// Defender refuses. Each one repeats steps of quilldrop.playbook.yaml, which
// tells the whole case and from which the ten beats read. fsagen has no include,
// so the repetition is the only way to share a step, and repetition drifts: a
// cookie edited in one file and not the other gives an attendee who built one
// topic a different answer from one who built the case.
//
// So every step of a topic must be a step of the case: the same actor, the same
// timing, and actions each identical to one the case step holds. The topic may
// leave actions out (registry keeps the hive and not the .reg export beside it)
// but may not change one. Its start, its jitter, its variables and its actors'
// bases must be the case's too, or the same text would mean something else.
func TestWorkshopTopicPlaybooksRepeatTheCase(t *testing.T) {
	evidence := filepath.Join(repositoryRoot, "examples", "workshop", "evidence")
	whole := readPlaybookForTopics(t, filepath.Join(evidence, "quilldrop.playbook.yaml"))
	topics, err := filepath.Glob(filepath.Join(evidence, "topics", "*.playbook.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	// The build scripts name these three; a fourth would be one they never build.
	if len(topics) != 3 {
		t.Fatalf("want the timestomp, ads and registry topic playbooks, found %d: %v", len(topics), topics)
	}

	wholeVars, _ := whole["variables"].(map[string]any)
	wholeActors := playbookActorBases(whole)
	wholeSteps, _ := whole["steps"].([]any)

	for _, path := range topics {
		name := filepath.Base(path)
		topic := readPlaybookForTopics(t, path)

		for key := range topic {
			switch key {
			case "start", "subsecond_jitter", "variables", "actors", "steps":
			default:
				t.Errorf("%s: top-level key %q is not one a topic may set", name, key)
			}
		}
		for _, key := range []string{"start", "subsecond_jitter"} {
			if !reflect.DeepEqual(topic[key], whole[key]) {
				t.Errorf("%s: %s is %v, the case has %v", name, key, topic[key], whole[key])
			}
		}
		vars, _ := topic["variables"].(map[string]any)
		for key, value := range vars {
			if !reflect.DeepEqual(value, wholeVars[key]) {
				t.Errorf("%s: variable %s is %v, the case has %v", name, key, value, wholeVars[key])
			}
		}
		for actor, base := range playbookActorBases(topic) {
			if wholeBase, ok := wholeActors[actor]; !ok || wholeBase != base {
				t.Errorf("%s: actor %s has base %q, the case has %q", name, actor, base, wholeBase)
			}
		}

		steps, _ := topic["steps"].([]any)
		if len(steps) == 0 {
			t.Errorf("%s: no steps", name)
		}
		for i, step := range steps {
			if !stepIsPartOfOne(step, wholeSteps) {
				t.Errorf("%s: step %d (%s) is not a step of quilldrop.playbook.yaml: "+
					"copy it from there unchanged, or change both", name, i+1, describeTopicStep(step))
			}
		}
	}
}

func readPlaybookForTopics(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var playbook map[string]any
	if err := yaml.Unmarshal(data, &playbook); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return playbook
}

func playbookActorBases(playbook map[string]any) map[string]any {
	bases := map[string]any{}
	actors, _ := playbook["actors"].([]any)
	for _, a := range actors {
		if actor, ok := a.(map[string]any); ok {
			bases[fmt.Sprint(actor["name"])] = actor["base"]
		}
	}
	return bases
}

// stepIsPartOfOne reports whether some case step has every key of step other
// than its actions, equal, and holds each of step's actions unchanged.
func stepIsPartOfOne(step any, wholeSteps []any) bool {
	s, ok := step.(map[string]any)
	if !ok {
		return false
	}
	actions, _ := s["actions"].([]any)
	if len(actions) == 0 {
		return false
	}
	for _, candidate := range wholeSteps {
		c, ok := candidate.(map[string]any)
		if !ok || len(c) != len(s) {
			continue
		}
		same := true
		for key, value := range s {
			if key != "actions" && !reflect.DeepEqual(value, c[key]) {
				same = false
				break
			}
		}
		if !same {
			continue
		}
		caseActions, _ := c["actions"].([]any)
		all := true
		for _, action := range actions {
			found := false
			for _, caseAction := range caseActions {
				if reflect.DeepEqual(action, caseAction) {
					found = true
					break
				}
			}
			if !found {
				all = false
				break
			}
		}
		if all {
			return true
		}
	}
	return false
}

func describeTopicStep(step any) string {
	s, _ := step.(map[string]any)
	actions, _ := s["actions"].([]any)
	if len(actions) > 0 {
		if first, ok := actions[0].(map[string]any); ok {
			return fmt.Sprintf("%v %v at %v", first["action"], first["path"], s["offset"])
		}
	}
	return fmt.Sprintf("at %v", s["offset"])
}
