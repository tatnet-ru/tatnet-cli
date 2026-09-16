package deploy

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Stats — что именно уехало, чтобы человек увидел это ДО выкладки.
type Stats struct {
	Files      int
	Bytes      int64    // суммарный размер файлов до сжатия
	Skipped    int      // исключено правилами
	SecretsHit []string // файлы окружения, не попавшие в архив
	IgnoreFile string   // по какому файлу правил шли
}

// ErrEmpty — в папке нечего выкладывать.
var ErrEmpty = fmt.Errorf("папка пуста после применения правил исключения")

// Pack пакует каталог в tar.gz.
//
// Пути в архиве — относительные и с прямыми слешами: архив распаковывается в
// Linux-госте, и путь из Windows с обратными слешами стал бы там ОДНИМ файлом
// с косой чертой в имени вместо дерева.
func Pack(dir string, w io.Writer, maxBytes int64) (Stats, error) {
	st := Stats{}
	ig, ignoreFile, err := LoadIgnore(dir)
	if err != nil {
		return st, err
	}
	st.IgnoreFile = ignoreFile

	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)

	var total int64
	walkErr := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		base := filepath.Base(rel)

		for _, skip := range alwaysExcluded {
			if base == skip {
				if info.IsDir() {
					return filepath.SkipDir
				}
				st.Skipped++
				return nil
			}
		}
		if !info.IsDir() && IsSecretFile(base) {
			// Не молча: человек узнаёт, что его .env остался дома, из вывода
			// команды, а не из того, что приложение не работает.
			st.SecretsHit = append(st.SecretsHit, rel)
			st.Skipped++
			return nil
		}
		if ig.Match(rel, info.IsDir()) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			st.Skipped++
			return nil
		}

		switch {
		case info.IsDir():
			return tw.WriteHeader(&tar.Header{
				Name: rel + "/", Mode: 0o755, Typeflag: tar.TypeDir,
			})
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(p)
			if err != nil {
				return err
			}
			// Симлинк наружу дерева в архиве бессмыслен: на той стороне цели
			// нет. Пропускаем с явным счётчиком, а не тащим битую ссылку.
			if filepath.IsAbs(target) || strings.HasPrefix(filepath.ToSlash(filepath.Clean(filepath.Join(filepath.Dir(rel), target))), "..") {
				st.Skipped++
				return nil
			}
			return tw.WriteHeader(&tar.Header{
				Name: rel, Mode: 0o777, Typeflag: tar.TypeSymlink, Linkname: filepath.ToSlash(target),
			})
		case info.Mode().IsRegular():
			total += info.Size()
			if maxBytes > 0 && total > maxBytes {
				return fmt.Errorf("исходники больше %d МиБ — исключите сборку и зависимости (.tatnetignore)", maxBytes/(1024*1024))
			}
			hdr := &tar.Header{
				Name: rel, Mode: int64(info.Mode().Perm()), Size: info.Size(),
				Typeflag: tar.TypeReg, ModTime: info.ModTime(),
			}
			if err := tw.WriteHeader(hdr); err != nil {
				return err
			}
			f, err := os.Open(p)
			if err != nil {
				return err
			}
			defer f.Close()
			n, err := io.Copy(tw, f)
			if err != nil {
				return err
			}
			if n != info.Size() {
				// Файл изменился под нами: tar с несовпавшим размером — битый
				// архив, и распаковщик на той стороне скажет об этом уже
				// внутри build-VM.
				return fmt.Errorf("%s изменился во время упаковки", rel)
			}
			st.Files++
			return nil
		default:
			// Сокеты, устройства, fifo: в исходниках проекта не значат ничего.
			st.Skipped++
			return nil
		}
	})
	if walkErr != nil {
		tw.Close()
		gz.Close()
		return st, walkErr
	}
	if err := tw.Close(); err != nil {
		gz.Close()
		return st, err
	}
	if err := gz.Close(); err != nil {
		return st, err
	}
	st.Bytes = total
	sort.Strings(st.SecretsHit)
	if st.Files == 0 {
		return st, ErrEmpty
	}
	return st, nil
}
