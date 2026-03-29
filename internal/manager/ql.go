package manager

import (
	"bufio"
	"os/exec"
	"strings"

	"github.com/neur0map/glazepkg/internal/model"
)

type LispImpl struct {
	Name string // e.g. sbcl, ccl, clisp, ecl, abcl
	Path string // $ which LispImpl.Name || wrapper
}

func (l LispImpl) BuildCmd(exprs ...string) *exec.Cmd {
	// ABCL is JVM-based
	if l.Name == "abcl" {
		args := []string{"-jar", l.Path}
		for _, e := range exprs {
			args = append(args, "--eval", e)
		}
		return exec.Command("java", args...)
	}

	args := []string{}
	for _, e := range exprs {
		args = append(args, "--eval", e)
	}

	args = append([]string{"--noinform", "--non-interactive"}, args...)

	return exec.Command(l.Path, args...)
}

type Quicklisp struct {
	Impl LispImpl
}

func (q *Quicklisp) Name() model.Source {
	return model.SourceQuicklisp
}

func (q *Quicklisp) Available() bool {
	return commandExists(q.Impl.Path)
}

func (q *Quicklisp) run(exprs ...string) ([]byte, error) {
	return q.Impl.BuildCmd(exprs...).Output()
}

func (q *Quicklisp) Scan() ([]model.Package, error) {
	out, err := q.run(
		"(require :asdf)",
		`(mapcar #'prin1-to-string (asdf:registered-systems))`,
	)

	if err != nil || len(out) == 0 {
		return nil, err
	}

	var pkgs []model.Package
	scanner := bufio.NewScanner(strings.NewReader(string(out)))

	for scanner.Scan() {
		name := strings.TrimSpace(scanner.Text())
		name = strings.Trim(name, "()\"")

		if name == "" {
			continue
		}

		pkgs = append(pkgs, model.Package{
			Name:   name,
			Source: model.SourceQuicklisp,
		})
	}

	return pkgs, nil
}

func (q *Quicklisp) Search(query string) ([]model.Package, error) {
	out, err := q.run(
		"(require :quicklisp)",
		`(ql:system-apropos "`+query+`")`,
	)

	if err != nil || len(out) == 0 {
		return nil, nil
	}

	var pkgs []model.Package
	scanner := bufio.NewScanner(strings.NewReader(string(out)))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		name := strings.Fields(line)[0]
		if name == "" {
			continue
		}

		pkgs = append(pkgs, model.Package{
			Name:        name,
			Description: line,
			Source:      model.SourceQuicklisp,
		})
	}

	return pkgs, nil
}

func (q *Quicklisp) InstallCmd(name string) *exec.Cmd {
	return q.Impl.BuildCmd(
		"(require :quicklisp)",
		`(ql:quickload "`+name+`")`,
	)
}

func (q *Quicklisp) RemoveCmd(name string) *exec.Cmd {
	return q.Impl.BuildCmd(
		"(require :quicklisp)",
		`(ignore-errors (ql:uninstall-system "`+name+`"))`,
	)
}

func (q *Quicklisp) UpgradeCmd(name string) *exec.Cmd {
	if name == "" {
		return q.Impl.BuildCmd(
			"(require :quicklisp)",
			"(ql:update-all-dists)",
		)
	}

	return q.Impl.BuildCmd(
		"(require :quicklisp)",
		`(ql:update-dist "`+name+`")`,
	)
}

func (q *Quicklisp) CheckUpdates(pkgs []model.Package) map[string]string {
	out, err := q.run(
		"(require :quicklisp)",
		"(ql:update-all-dists :prompt nil)",
	)

	if err != nil || len(out) == 0 {
		return nil
	}

	updates := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(string(out)))

	for scanner.Scan() {
		line := scanner.Text()

		if strings.Contains(line, "Updating") {
			fields := strings.Fields(line)
			if len(fields) > 0 {
				name := fields[len(fields)-1]
				updates[name] = "latest"
			}
		}
	}

	return updates
}

func (q *Quicklisp) Describe(pkgs []model.Package) map[string]string {
	descs := make(map[string]string)

	for _, pkg := range pkgs {
		out, err := q.run(
			"(require :asdf)",
			`(let ((sys (asdf:find-system "`+pkg.Name+`" nil)))
			 (when sys
			   (format t "~a" (asdf:system-description sys))))`,
		)

		if err != nil || len(out) == 0 {
			continue
		}

		descs[pkg.Name] = strings.TrimSpace(string(out))
	}

	return descs
}

func (q *Quicklisp) ListDependencies(pkgs []model.Package) map[string][]string {
	deps := make(map[string][]string)

	for _, pkg := range pkgs {
		out, err := q.run(
			"(require :asdf)",
			`(let ((sys (asdf:find-system "`+pkg.Name+`" nil)))
			 (when sys
			   (asdf:system-depends-on sys)))`,
		)

		if err != nil || len(out) == 0 {
			continue
		}

		var pkgDeps []string
		scanner := bufio.NewScanner(strings.NewReader(string(out)))

		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			line = strings.Trim(line, "()")

			for _, dep := range strings.Fields(line) {
				dep = strings.Trim(dep, "\"")
				if dep != "" {
					pkgDeps = append(pkgDeps, dep)
				}
			}
		}

		deps[pkg.Name] = pkgDeps
	}

	return deps
}

