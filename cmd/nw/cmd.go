package main

import (
	"github.com/spf13/cobra"
)

type commandFunc func() *cobra.Command

type cli struct {
	cmds []commandFunc
}

func NewCommand() *cli {
	return &cli{
		cmds: make([]commandFunc, 0),
	}
}

func (c *cli) Register(cmds ...commandFunc) {
	c.cmds = append(c.cmds, cmds...)
}

func (c *cli) Execute() {
	rootCmd := &cobra.Command{
		Use:   "t0",
		Short: "Tndr0cean CLI",
		Long:  "Tndr0cean Web framework & Generator",
	}

	for _, command := range c.cmds {
		rootCmd.AddCommand(command())
	}

	rootCmd.Execute()
}
