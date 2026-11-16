package cmd

import (
	"fmt"
	"strings"

	"github.com/eldius/cli-telemetry-poc/internal/client"
	"github.com/eldius/cli-telemetry-poc/internal/client/firecrawl"
	"github.com/spf13/cobra"
)

// searchCmd represents the search command
var searchCmd = &cobra.Command{
	Use:   "search",
	Short: "Search the internet",
	Long:  `Search the internet.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		c := client.NewHTTPClient()

		res, err := firecrawl.Search(cmd.Context(), c, strings.Join(args, " "))
		if err != nil {
			fmt.Println(err)
			return err
		}

		cmd.Println("Search results:")
		for _, r := range res.Data.Web {
			fmt.Println("-----------------------")
			fmt.Printf("%03d - %s\n", r.Position, r.Title)
			fmt.Println(r.Url)
			fmt.Println(r.Description)
			fmt.Println("-----------------------")
			fmt.Println()
			fmt.Println()
		}
		cmd.Println(res)

		return nil
	},
}

func init() {
	rootCmd.AddCommand(searchCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// searchCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// searchCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
