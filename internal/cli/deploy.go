package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/tatnet-ru/tatnet-go/tatnet"

	"github.com/tatnet-ru/tatnet-cli/internal/deploy"
	"github.com/tatnet-ru/tatnet-cli/internal/output"
)

// linkDir/linkFile — привязка папки к аппу, как `.vercel/project.json` у
// Vercel. Второй запуск `tatnet deploy` в той же папке обязан попасть в ТОТ
// ЖЕ апп: иначе каждая выкладка плодила бы новое приложение с новым адресом.
const (
	linkDir  = ".tatnet"
	linkFile = "project.json"
)

type link struct {
	ProjectID string `json:"project_id"`
	AppID     string `json:"app_id"`
	AppName   string `json:"app_name,omitempty"`
}

func readLink(dir string) (*link, error) {
	data, err := os.ReadFile(filepath.Join(dir, linkDir, linkFile))
	if err != nil {
		return nil, err
	}
	var l link
	if err := json.Unmarshal(data, &l); err != nil {
		return nil, fmt.Errorf("%s/%s повреждён: %w", linkDir, linkFile, err)
	}
	return &l, nil
}

func writeLink(dir string, l link) error {
	d := filepath.Join(dir, linkDir)
	if err := os.MkdirAll(d, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(d, linkFile), append(data, '\n'), 0o644)
}

func newDeployCommand(env *Env) *cobra.Command {
	var (
		appRef, name                            string
		framework, buildCmd, installCmd, outDir string
		rootDir                                 string
		maxMiB                                  int
		noCreate, showLogs, noWait              bool
	)

	cmd := &cobra.Command{
		Use:   "deploy [папка]",
		Short: "Выложить приложение из папки на диске",
		Long: "Собирает и разворачивает то, что лежит в папке, без репозитория.\n\n" +
			"Папка пакуется и уезжает на сборку целиком. В архив НЕ попадают\n" +
			"`.git`, `node_modules`, служебный `.tatnet` и файлы окружения\n" +
			"(`.env`, `.env.*`, кроме `.env.example`) — переменные задаются\n" +
			"командой `tatnet app env`, а не случайно вместе с папкой.\n" +
			"Остальное исключается правилами `.tatnetignore`, а если его нет —\n" +
			"`.gitignore`.\n\n" +
			"Первый запуск создаёт приложение и запоминает его в\n" +
			"`.tatnet/project.json`; следующие выкладывают в то же самое.\n\n" +
			"В stdout уходит ТОЛЬКО адрес — всё остальное в stderr, поэтому\n" +
			"`url=$(tatnet deploy)` работает без разбора вывода.\n\n" +
			"У выкладки из папки нет коммита: происхождение сборки — отпечаток\n" +
			"архива, он виден в `tatnet app build list`.",
		Annotations: ops("apps_create_deployment", "apps_stream_build_logs"),
		Args:        cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}
			abs, err := filepath.Abs(dir)
			if err != nil {
				return err
			}
			if info, err := os.Stat(abs); err != nil || !info.IsDir() {
				return fmt.Errorf("%s — не папка", dir)
			}

			c, err := env.Client()
			if err != nil {
				return err
			}
			saved, _ := readLink(abs)

			project, err := env.deployProject(cmd.Context(), saved)
			if err != nil {
				return err
			}

			appID, created, err := env.deployApp(cmd.Context(), c, project, saved, deployAppOpts{
				ref: appRef, name: name, dirName: filepath.Base(abs), noCreate: noCreate,
				framework: optStr(cmd, "framework", framework),
				build:     optStr(cmd, "build-command", buildCmd),
				install:   optStr(cmd, "install-command", installCmd),
				output:    optStr(cmd, "output-directory", outDir),
				root:      optStr(cmd, "root-directory", rootDir),
			})
			if err != nil {
				return err
			}

			// Привязка пишется СРАЗУ после создания, до выкладки: сборка может
			// не задаться, а приложение уже существует — без записи следующий
			// запуск создал бы второе.
			if created {
				if err := writeLink(abs, link{ProjectID: project, AppID: appID, AppName: name}); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "предупреждение: не удалось записать %s/%s: %v\n", linkDir, linkFile, err)
				}
			}

			buildID, err := env.uploadFolder(cmd, c, project, appID, abs, int64(maxMiB)*1024*1024)
			if err != nil {
				return err
			}

			if noWait {
				fmt.Fprintf(cmd.ErrOrStderr(), "Сборка поставлена в очередь: %s\n", buildID)
				return env.printAppURL(cmd, c, project, appID)
			}

			if err := env.awaitBuild(cmd, c, project, appID, buildID, showLogs); err != nil {
				return err
			}
			return env.printAppURL(cmd, c, project, appID)
		},
	}

	f := cmd.Flags()
	f.StringVar(&appRef, "app", "", "приложение: имя или id (иначе — из .tatnet/project.json)")
	f.StringVar(&name, "name", "", "имя нового приложения (по умолчанию — имя папки)")
	f.BoolVar(&noCreate, "no-create", false, "не создавать приложение, если его нет")
	f.BoolVar(&showLogs, "logs", false, "печатать лог сборки")
	f.BoolVar(&noWait, "no-wait", false, "не ждать окончания сборки")
	f.IntVar(&maxMiB, "max-size", 100, "предел размера исходников, МиБ")
	f.StringVar(&framework, "framework", "", "фреймворк (при создании приложения)")
	f.StringVar(&buildCmd, "build-command", "", "команда сборки (при создании)")
	f.StringVar(&installCmd, "install-command", "", "команда установки зависимостей (при создании)")
	f.StringVar(&outDir, "output-directory", "", "каталог с результатом сборки (при создании)")
	f.StringVar(&rootDir, "root-directory", "", "подкаталог проекта (при создании)")
	return cmd
}

// deployProject: флаг и переменная важнее привязки — человек сказал явно.
func (e *Env) deployProject(ctx context.Context, saved *link) (string, error) {
	if strings.TrimSpace(e.Project) == "" && saved != nil && saved.ProjectID != "" {
		return e.ResolveProject(ctx, saved.ProjectID)
	}
	return e.RequireProject(ctx)
}

type deployAppOpts struct {
	ref, name, dirName                      string
	framework, build, install, output, root *string
	noCreate                                bool
}

// deployApp находит приложение или создаёт новое.
func (e *Env) deployApp(ctx context.Context, c *tatnet.ClientWithResponses, project string, saved *link, o deployAppOpts) (string, bool, error) {
	if o.ref != "" {
		id, err := resolveRef(ctx, "приложение", o.ref, func(ctx context.Context) ([]any, error) {
			return e.listApps(ctx, project, 0)
		}, "name")
		return id, false, err
	}
	if saved != nil && saved.AppID != "" {
		// Проверяем, что апп ещё жив: удалённый апп дал бы 404 на выкладке, и
		// причина «а он удалён» была бы не видна.
		if _, err := call(c.AppsGetAppWithResponse(ctx, project, saved.AppID)); err == nil {
			return saved.AppID, false, nil
		}
		fmt.Fprintf(os.Stderr, "приложение из %s/%s больше не существует — создаю новое\n", linkDir, linkFile)
	}
	if o.noCreate {
		return "", false, fmt.Errorf("приложение не задано и не найдено в %s/%s, а --no-create запрещает создавать новое", linkDir, linkFile)
	}

	name := o.name
	if name == "" {
		name = sanitizeAppName(o.dirName)
	}
	upload := "upload"
	v, err := call(c.AppsCreateAppWithResponse(ctx, project, tatnet.V1AppCreate{
		Name:            name,
		SourceType:      &upload,
		Framework:       o.framework,
		BuildCommand:    o.build,
		InstallCommand:  o.install,
		OutputDirectory: o.output,
		RootDirectory:   o.root,
	}))
	if err != nil {
		return "", false, err
	}
	id := output.Value(v, "id")
	if id == "" {
		return "", false, fmt.Errorf("API не вернул id созданного приложения")
	}
	fmt.Fprintf(os.Stderr, "Создано приложение %q\n", name)
	return id, true, nil
}

// sanitizeAppName делает из имени папки имя приложения.
func sanitizeAppName(dir string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(dir) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			b.WriteRune('-')
		}
	}
	name := strings.Trim(b.String(), "-")
	if name == "" {
		name = "app"
	}
	return name
}

// uploadFolder пакует папку во временный файл и отправляет её.
//
// Во временный файл, а не в память: размер задаёт содержимое папки клиента, и
// собирать его целиком в RAM значит однажды упасть на чужом монорепозитории.
func (e *Env) uploadFolder(cmd *cobra.Command, c *tatnet.ClientWithResponses, project, appID, dir string, maxBytes int64) (string, error) {
	tmp, err := os.CreateTemp("", "tatnet-deploy-*.tar.gz")
	if err != nil {
		return "", err
	}
	defer func() {
		name := tmp.Name()
		tmp.Close()
		os.Remove(name)
	}()

	st, err := deploy.Pack(dir, tmp, maxBytes)
	if err != nil {
		if err == deploy.ErrEmpty {
			return "", fmt.Errorf("в папке нечего выкладывать: всё исключено правилами (%s)", st.IgnoreFile)
		}
		return "", err
	}
	size, err := tmp.Seek(0, 2)
	if err != nil {
		return "", err
	}
	if _, err := tmp.Seek(0, 0); err != nil {
		return "", err
	}

	errOut := cmd.ErrOrStderr()
	rules := "без файла правил"
	if st.IgnoreFile != "" {
		rules = "правила: " + st.IgnoreFile
	}
	fmt.Fprintf(errOut, "Упаковано %d файлов (%s в архиве, %s), %s\n",
		st.Files, humanBytes(size), rules, pluralSkipped(st.Skipped))
	for _, s := range st.SecretsHit {
		fmt.Fprintf(errOut, "  пропущен %s — переменные задаются `tatnet app env`, а не файлом в архиве\n", s)
	}

	resp, err := call(c.AppsCreateDeploymentWithBodyWithResponse(
		cmd.Context(), project, appID, "application/gzip", tmp))
	if err != nil {
		return "", err
	}
	buildID := output.Value(resp, "id")
	if buildID == "" {
		return "", fmt.Errorf("API не вернул идентификатор сборки")
	}
	fmt.Fprintf(errOut, "Исходники загружены, сборка %s\n", buildID)
	return buildID, nil
}

func pluralSkipped(n int) string {
	if n == 0 {
		return "ничего не исключено"
	}
	return fmt.Sprintf("исключено %d", n)
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d Б", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cиБ", float64(n)/float64(div), []rune("КМГТ")[exp])
}

// awaitBuild ждёт окончания сборки, при --logs печатая её лог.
func (e *Env) awaitBuild(cmd *cobra.Command, c *tatnet.ClientWithResponses, project, appID, buildID string, showLogs bool) error {
	errOut := cmd.ErrOrStderr()
	shown := 0
	if showLogs {
		n, err := e.streamBuildLog(cmd, c, project, appID, buildID, 0)
		shown = n
		if err != nil {
			fmt.Fprintf(errOut, "предупреждение: лог сборки прервался: %v\n", err)
		}
	} else {
		fmt.Fprintln(errOut, "Идёт сборка... (лог: --logs)")
	}

	// Статус спрашивается у API и ПОСЛЕ лога: конец потока логов означает
	// «лог кончился», а не «сборка удалась». Это разные вопросы.
	deadline := time.Now().Add(30 * time.Minute)
	for time.Now().Before(deadline) {
		status, errText, err := e.buildStatus(cmd.Context(), c, project, appID, buildID)
		if err != nil {
			return err
		}
		switch status {
		case "success":
			e.printMissedLog(cmd, c, project, appID, buildID, showLogs, shown)
			fmt.Fprintln(errOut, "Сборка прошла")
			return nil
		case "error", "failed", "cancelled":
			e.printMissedLog(cmd, c, project, appID, buildID, showLogs, shown)
			if errText != "" {
				return fmt.Errorf("сборка не прошла: %s", errText)
			}
			return fmt.Errorf("сборка не прошла (%s)", status)
		}
		select {
		case <-cmd.Context().Done():
			return cmd.Context().Err()
		case <-time.After(2 * time.Second):
		}
	}
	return fmt.Errorf("сборка не завершилась за 30 минут; проверьте: tatnet app build list --app %s", appID)
}

func (e *Env) buildStatus(ctx context.Context, c *tatnet.ClientWithResponses, project, appID, buildID string) (string, string, error) {
	limit := 20
	v, err := call(c.AppsListBuildsWithResponse(ctx, project, appID, &tatnet.AppsListBuildsParams{Limit: &limit}))
	if err != nil {
		return "", "", err
	}
	for _, item := range items(v) {
		if output.Value(item, "id") == buildID {
			return output.Value(item, "status"), output.Value(item, "error"), nil
		}
	}
	// Сборки ещё нет в списке — она только что заведена.
	return "queued", "", nil
}

// printMissedLog дочитывает лог ПОСЛЕ окончания сборки.
//
// Живой поток закрывается, как только сборка стала терминальной, — а строки
// в этот момент ещё едут: короткая сборка (2,3 с у статики) успевает
// закончиться раньше, чем доедет её собственный вывод. Замер 16.09: клиент
// показал 3 строки из 16 и написал «Сборка прошла». Повторный запрос у
// завершённой сборки отдаёт ПОЛНУЮ историю, поэтому пропускаем столько
// строк, сколько уже напечатали, и печатаем хвост.
func (e *Env) printMissedLog(cmd *cobra.Command, c *tatnet.ClientWithResponses, project, appID, buildID string, showLogs bool, shown int) {
	if !showLogs {
		return
	}
	if _, err := e.streamBuildLog(cmd, c, project, appID, buildID, shown); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "предупреждение: хвост лога дочитать не удалось: %v\n", err)
	}
}

// streamBuildLog печатает лог сборки, пропуская первые skip строк.
// Возвращает, сколько строк лога прошло через него всего (включая пропущенные).
func (e *Env) streamBuildLog(cmd *cobra.Command, c *tatnet.ClientWithResponses, project, appID, buildID string, skip int) (int, error) {
	resp, err := c.AppsStreamBuildLogs(cmd.Context(), project, appID, buildID)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return 0, fmt.Errorf("лог недоступен: %s", resp.Status)
	}

	errOut := cmd.ErrOrStderr()
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	seen := 0
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "data: "):
			seen++
			if seen > skip {
				fmt.Fprintln(errOut, "  "+strings.TrimPrefix(line, "data: "))
			}
		case line == "event: done":
			return seen, nil
		}
	}
	return seen, sc.Err()
}

// printAppURL печатает адрес приложения — и ТОЛЬКО его — в stdout.
func (e *Env) printAppURL(cmd *cobra.Command, c *tatnet.ClientWithResponses, project, appID string) error {
	limit := 100
	v, err := call(c.AppsListDomainsWithResponse(cmd.Context(), project, appID, &tatnet.AppsListDomainsParams{Limit: &limit}))
	if err != nil {
		return err
	}
	all := items(v)
	best := ""
	for _, d := range all {
		domain := output.Value(d, "domain")
		if domain == "" {
			continue
		}
		// Именно Bool, а не сравнение строки: Value форматирует для человека
		// и отдаёт «да», поэтому сравнение с "true" не совпало бы никогда.
		if output.Bool(d, "is_default") {
			best = domain
			break
		}
		if best == "" {
			best = domain
		}
	}
	if best == "" {
		fmt.Fprintln(cmd.ErrOrStderr(), "У приложения пока нет домена")
		return nil
	}
	fmt.Fprintln(cmd.OutOrStdout(), "https://"+best)
	return nil
}
