// Package deploy упаковывает папку проекта в архив, который уезжает на сборку.
package deploy

import (
	"bufio"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Всегда исключается, независимо от правил игнора.
//
// `.git` и `node_modules` — это вес, которому нечего делать в исходниках:
// зависимости ставит сборка, история ей не нужна. `.tatnet` — наш служебный
// каталог. Файлы окружения исключены НАМЕРЕННО и жёстко: деплой из папки
// поднимает на прод то, что лежит на диске, и `.env` с боевыми ключами
// уезжает туда первым. Переменные задаются `tatnet app env`, а не случайно.
var alwaysExcluded = []string{
	".git",
	".tatnet",
	"node_modules",
}

// IsSecretFile — файл окружения, который не должен уехать на сервер.
// `.env.example` и `.env.sample` — шаблоны без значений, они остаются.
func IsSecretFile(name string) bool {
	if !strings.HasPrefix(name, ".env") {
		return false
	}
	switch name {
	case ".env.example", ".env.sample", ".env.template":
		return false
	}
	return true
}

// Ignore — набор правил исключения в стиле .gitignore.
//
// Поддерживается подмножество: комментарии, пустые строки, отрицание «!»,
// привязка к корню ведущей косой чертой, «только каталог» завершающей,
// «*» и «?» внутри сегмента, «**» через сегменты. Этого хватает для
// подавляющего большинства настоящих .gitignore; чего тут нет — редкие
// конструкции вроде диапазонов [a-z] в именах.
type Ignore struct {
	rules []rule
}

type rule struct {
	pattern  string
	negate   bool
	dirOnly  bool
	anchored bool
}

// LoadIgnore читает правила из каталога: `.tatnetignore`, если он есть,
// иначе `.gitignore`.
//
// Именно «иначе», а не «оба»: два действующих файла означали бы, что человек,
// написавший `.tatnetignore`, всё равно не знает, что уедет — правила
// складывались бы с невидимыми ему.
func LoadIgnore(dir string) (*Ignore, string, error) {
	for _, name := range []string{".tatnetignore", ".gitignore"} {
		f, err := os.Open(filepath.Join(dir, name))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, "", err
		}
		defer f.Close()

		ig := &Ignore{}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			ig.add(sc.Text())
		}
		if err := sc.Err(); err != nil {
			return nil, "", err
		}
		return ig, name, nil
	}
	return &Ignore{}, "", nil
}

func (ig *Ignore) add(line string) {
	line = strings.TrimRight(line, " \t")
	if line == "" || strings.HasPrefix(line, "#") {
		return
	}
	r := rule{}
	if strings.HasPrefix(line, "!") {
		r.negate = true
		line = line[1:]
	}
	if strings.HasSuffix(line, "/") {
		r.dirOnly = true
		line = strings.TrimSuffix(line, "/")
	}
	if strings.HasPrefix(line, "/") {
		r.anchored = true
		line = strings.TrimPrefix(line, "/")
	} else if strings.Contains(line, "/") {
		// Правило с косой чертой внутри привязано к корню — так же, как в git.
		r.anchored = true
	}
	if line == "" {
		return
	}
	r.pattern = line
	ig.rules = append(ig.rules, r)
}

// Match отвечает, исключён ли путь (относительный, с прямыми слешами).
//
// Последнее совпавшее правило побеждает — как в git: иначе «!» не смог бы
// вернуть исключённое.
func (ig *Ignore) Match(rel string, isDir bool) bool {
	excluded := false
	for _, r := range ig.rules {
		if r.dirOnly && !isDir {
			continue
		}
		if r.match(rel, isDir) {
			excluded = !r.negate
		}
	}
	return excluded
}

func (r rule) match(rel string, isDir bool) bool {
	if r.anchored {
		return matchPath(r.pattern, rel)
	}
	// Непривязанное правило проверяется на самом пути и на каждом его хвосте:
	// «build» в .gitignore исключает и «build», и «web/build».
	for i := 0; i <= len(rel); i++ {
		if i == 0 || rel[i-1] == '/' {
			if matchPath(r.pattern, rel[i:]) {
				return true
			}
		}
	}
	return false
}

// matchPath сопоставляет шаблон с путём посегментно, понимая «**».
func matchPath(pattern, name string) bool {
	if strings.Contains(pattern, "**") {
		return matchDoubleStar(strings.Split(pattern, "/"), strings.Split(name, "/"))
	}
	pp := strings.Split(pattern, "/")
	np := strings.Split(name, "/")
	if len(pp) > len(np) {
		return false
	}
	for i, seg := range pp {
		ok, err := path.Match(seg, np[i])
		if err != nil || !ok {
			return false
		}
	}
	// Шаблон короче пути — совпал каталог, а значит и всё под ним.
	return true
}

func matchDoubleStar(pp, np []string) bool {
	if len(pp) == 0 {
		return true
	}
	if pp[0] == "**" {
		for i := 0; i <= len(np); i++ {
			if matchDoubleStar(pp[1:], np[i:]) {
				return true
			}
		}
		return false
	}
	if len(np) == 0 {
		return false
	}
	ok, err := path.Match(pp[0], np[0])
	if err != nil || !ok {
		return false
	}
	if len(pp) == 1 {
		return true
	}
	return matchDoubleStar(pp[1:], np[1:])
}
