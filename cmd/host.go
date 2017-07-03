package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"io/ioutil"
	"github.com/pkg/errors"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"log"
	"github.com/glassechidna/lastkeypair/common"
	"github.com/aws/aws-sdk-go/aws"
	"encoding/json"
	"github.com/aws/aws-sdk-go/service/lambda"
	"os"
)

var hostCmd = &cobra.Command{
	Use:   "host",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		hostKeyPath, _ := cmd.PersistentFlags().GetString("host-key-path")
		signedHostKeyPath, _ := cmd.PersistentFlags().GetString("signed-host-key-path")
		caPubkeyPath, _ := cmd.PersistentFlags().GetString("cert-authority-path")
		sshdConfigPath, _ := cmd.PersistentFlags().GetString("sshd-config-path")
		functionName, _ := cmd.PersistentFlags().GetString("lambda-name")
		kmsKeyId, _ := cmd.PersistentFlags().GetString("kms-key")
		funcIdentity, _ := cmd.PersistentFlags().GetString("func-identity")

		err := doit(hostKeyPath, signedHostKeyPath, caPubkeyPath, sshdConfigPath, functionName, kmsKeyId, funcIdentity)
		if err != nil {
			log.Panicf("err: %s\n", err.Error())
		}
	},
}

func doit(hostKeyPath, signedHostKeyPath, caPubkeyPath, sshdConfigPath, functionName, kmsKeyId, funcIdentity string) error {
	hostKeyBytes, err := ioutil.ReadFile(hostKeyPath)
	if err != nil {
		return errors.Wrap(err, "reading ssh host key")
	}
	hostKey := string(hostKeyBytes)

	sessOpts := session.Options{
		SharedConfigState: session.SharedConfigEnable,
		AssumeRoleTokenProvider: stscreds.StdinTokenProvider,
	}

	sess, err := session.NewSessionWithOptions(sessOpts)
	if err != nil {
		return errors.Wrap(err, "creating aws session")
	}

	ident, err := common.CallerIdentityUser(sess)
	instanceArn, err := getInstanceArn(sess)
	token, err := hostCertToken(sess, *ident, kmsKeyId, funcIdentity, *instanceArn)

	client := ec2metadata.New(sess)
	caPubkey, err := client.GetMetadata("public-keys/0/openssh-key")
	if err != nil {
		return errors.Wrap(err, "fetching ssh CA key")
	}

	response, err := requestSignedHostKey(sess, functionName, common.HostCertReqJson{
		EventType: "HostCertReq",
		Token: *token,
		PublicKey: hostKey,
	})

	err = ioutil.WriteFile(signedHostKeyPath, []byte(response.SignedHostPublicKey), 0600)
	if err != nil {
		return errors.Wrap(err, "writing signed host key to filesystem")
	}

	err = ioutil.WriteFile(caPubkeyPath, []byte(caPubkey), 0600)
	if err != nil {
		return errors.Wrap(err, "writing ca pubkey to filesystem")
	}

	err = appendToFile(sshdConfigPath, fmt.Sprintf(`
HostCertificate %s
TrustedUserCAKeys %s
`, signedHostKeyPath, caPubkeyPath))
	if err != nil {
		return errors.Wrap(err, "appending to sshd config")
	}

	return nil
}

func getInstanceArn(sess *session.Session) (*string, error) {
	client := ec2metadata.New(sess)

	region, err := client.Region()
	if err != nil {
		return nil, errors.Wrap(err, "getting region")
	}

	ident, err := client.GetInstanceIdentityDocument()
	if err != nil {
		return nil, errors.Wrap(err, "getting identity doc for account id and instance id")
	}

	ret := fmt.Sprintf("arn:aws:ec2:%s:%s:instance/%s", region, ident.AccountID, ident.InstanceID)
	return &ret, nil

}

func requestSignedHostKey(sess *session.Session, functionName string, request common.HostCertReqJson) (*common.HostCertRespJson, error) {
	payload, err := json.Marshal(&request)
	if err != nil {
		return nil, errors.Wrap(err, "couldn't serialise host cert req json")
	}

	client := lambda.New(sess)

	input := lambda.InvokeInput{
		FunctionName: aws.String(functionName),
		Payload: payload,
	}

	resp, err := client.Invoke(&input)
	if err != nil {
		return nil, errors.Wrap(err, "invoking CA lambda")
	}

	response := common.HostCertRespJson{}
	err = json.Unmarshal(resp.Payload, &response)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshalling lambda resp payload")
	}

	return &response, nil
}

func hostCertToken(sess *session.Session, ident common.StsIdentity, kmsKeyId, funcIdentity, instanceArn string) (*common.Token, error) {
	params := common.TokenParams{
		KeyId:           kmsKeyId,
		FromId:          ident.UserId,
		FromAccount:     ident.AccountId,
		To:              funcIdentity,
		Type:            "AssumedRole",
		HostInstanceArn: instanceArn,
	}

	ret := common.CreateToken(sess, params)
	return &ret, nil
}

func appendToFile(path, text string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, os.ModeAppend)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(text)
	if err != nil {
		return err
	}
	return nil
}

func init() {
	RootCmd.AddCommand(hostCmd)

	hostCmd.PersistentFlags().String("host-key-path", "/etc/ssh/ssh_host_rsa_key.pub", "")
	hostCmd.PersistentFlags().String("signed-host-key-path", "/etc/ssh/ssh_host_rsa_key-cert.pub", "")
	hostCmd.PersistentFlags().String("cert-authority-path", "/etc/ssh/cert_authority.pub", "")
	hostCmd.PersistentFlags().String("sshd-config-path", "/etc/ssh/sshd_config", "")
	hostCmd.PersistentFlags().String("lambda-name", "LastKeypair", "")
	hostCmd.PersistentFlags().String("func-identity", "LastKeypair", "")
	hostCmd.PersistentFlags().String("kms-key", "alias/LastKeypair", "ID, ARN or alias of KMS key for auth to CA")
}
