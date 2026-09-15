package cli

import "github.com/spf13/cobra"

// Необязательные поля тела запроса — указатели: «не передавали» и «передали
// нулевое значение» это разные намерения, и PATCH их различает. Поэтому
// указатель ставится по факту наличия флага, а не по значению.

func optStr(cmd *cobra.Command, name string, v string) *string {
	if cmd.Flags().Changed(name) {
		return &v
	}
	return nil
}

func optInt(cmd *cobra.Command, name string, v int) *int {
	if cmd.Flags().Changed(name) {
		return &v
	}
	return nil
}

func optBool(cmd *cobra.Command, name string, v bool) *bool {
	if cmd.Flags().Changed(name) {
		return &v
	}
	return nil
}

func optStrSlice(cmd *cobra.Command, name string, v []string) *[]string {
	if cmd.Flags().Changed(name) {
		return &v
	}
	return nil
}

func ptr[T any](v T) *T { return &v }
