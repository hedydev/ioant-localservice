package web

import (
	"strings"
	"testing"
)

func TestProjectAwareSPARouting(t *testing.T) {
	routerRaw, err := Files.ReadFile("js/router.js")
	if err != nil {
		t.Fatal(err)
	}
	router := string(routerRaw)
	for _, required := range []string{
		"#/projects/",
		"#/services",
		"export function readRoute()",
		"export function writeRoute(",
	} {
		if !strings.Contains(router, required) {
			t.Fatalf("router.js missing %q", required)
		}
	}

	projectsRaw, err := Files.ReadFile("js/projects.js")
	if err != nil {
		t.Fatal(err)
	}
	projects := string(projectsRaw)
	for _, required := range []string{
		"export async function selectProject(",
		"updateProjectSelection()",
		"void selectProject(next,{view:targetView});",
		"renderedProjectsSignature",
	} {
		if !strings.Contains(projects, required) {
			t.Fatalf("projects.js missing incremental routing marker %q", required)
		}
	}
	clickStart := strings.Index(projects, "$('#projects').addEventListener('click'")
	clickEnd := strings.Index(projects[clickStart:], "$('#new-project').onclick")
	if clickStart < 0 || clickEnd < 0 {
		t.Fatal("project click handler not found")
	}
	clickHandler := projects[clickStart : clickStart+clickEnd]
	if strings.Contains(clickHandler, "refreshData()") || strings.Contains(clickHandler, "innerHTML=") {
		t.Fatal("project switching must not refetch the project list or rebuild the sidebar")
	}

	navigationRaw, err := Files.ReadFile("js/navigation.js")
	if err != nil {
		t.Fatal(err)
	}
	navigation := string(navigationRaw)
	for _, required := range []string{
		"readRoute()",
		"selectProject(route.project,{updateRoute:false})",
		"window.addEventListener('hashchange'",
		"adminDenied&&!adminResolved",
	} {
		if !strings.Contains(navigation, required) {
			t.Fatalf("navigation.js missing route behavior %q", required)
		}
	}
}
