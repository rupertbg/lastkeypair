// Copyright © 2017 NAME HERE <EMAIL ADDRESS>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/glassechidna/lastkeypair/common"
	"github.com/aws/aws-sdk-go/service/sts"
	"log"
	"fmt"
	"encoding/json"
)

// tokenCreateCmd represents the tokenCreate command
var tokenCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		// TODO: Work your own magic here
		profile := viper.GetString("profile")
		region := viper.GetString("region")
		sess := common.AwsSession(profile, region)

		key := viper.GetString("kms-key")
		from := viper.GetString("from")
		fromAcct := viper.GetString("from-account")
		to := viper.GetString("to")
		typ := viper.GetString("principal")

		params := common.TokenParams{
			KeyId: key,
			From: from,
			FromAccount: fromAcct,
			To: to,
			Type: typ,
		}

		stsClient := sts.New(sess)
		stsAcct, stsFrom, err := common.CallerIdentityUser(stsClient)
		if err != nil {
			log.Panicf("No 'from' specified and could not determine caller identity: %s", err.Error())
		}

		if len(params.From) == 0 {
			params.From = *stsFrom
		}
		if len(params.FromAccount) == 0 {
			params.FromAccount = *stsAcct
		}

		token := common.CreateToken(sess, params)
		jsonToken, _ := json.Marshal(token)
		fmt.Println(string(jsonToken))
	},
}

func init() {
	tokenCmd.AddCommand(tokenCreateCmd)

	tokenCreateCmd.PersistentFlags().String("profile", "", "")
	tokenCreateCmd.PersistentFlags().String("region", "", "")

	tokenCreateCmd.PersistentFlags().String("kms-key", "alias/LastKeypair", "ID, ARN or alias of KMS key for auth to CA")
	tokenCreateCmd.PersistentFlags().String("from-account", "", "AWS account of 'from' user")
	tokenCreateCmd.PersistentFlags().String("from", "", "(defaults to IAM username)")
	tokenCreateCmd.PersistentFlags().String("to", "", "")
	tokenCreateCmd.PersistentFlags().String("principal", "user", "")

	viper.BindPFlags(tokenCreateCmd.PersistentFlags())
}
