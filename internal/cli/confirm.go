package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// ErrAborted — пользователь не подтвердил разрушающее действие.
var ErrAborted = fmt.Errorf("отменено")

// addYes вешает на команду флаг подтверждения.
func addYes(cmd *cobra.Command, yes *bool) {
	cmd.Flags().BoolVarP(yes, "yes", "y", false, "не спрашивать подтверждения")
}

// confirm спрашивает подтверждение разрушающего действия.
//
// Когда ввод не терминал (скрипт, CI), вопрос задать некому: мы не молчаливо
// соглашаемся, а требуем явный --yes. Иначе `tatnet vm delete` в пайплайне
// удалял бы ВМ, «ответив» за человека.
func confirm(cmd *cobra.Command, yes bool, format string, args ...any) error {
	if yes {
		return nil
	}
	question := fmt.Sprintf(format, args...)
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return fmt.Errorf("%s\nВвод не интерактивный — подтвердите флагом --yes", question)
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "%s [y/N]: ", question)
	line, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	if err != nil {
		return ErrAborted
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes", "д", "да":
		return nil
	}
	return ErrAborted
}
