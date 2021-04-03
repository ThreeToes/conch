package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/ec2instanceconnect"
	"github.com/AlecAivazis/survey/v2"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"path"
	"time"
)

var debug *bool

func main() {
	portF := flag.Int("o", 22, "SSH port on the server")
	profileF := flag.String("p", "default", "AWS Profile")
	regionF := flag.String("r", "ap-southeast-2", "AWS Region")
	userF := flag.String("u", "ec2-user", "Server user")
	debug = flag.Bool("v", false, "Verbose output")
	flag.Parse()

	var creds []func(options *config.LoadOptions) error
	if *profileF != "" {
		logMsg("Using profile %s", *profileF)
		creds = append(creds, config.WithSharedConfigProfile(*profileF))
	}
	logMsg("Using region %s", *regionF)
	creds = append(creds, config.WithRegion(*regionF))
	cfg, err := config.LoadDefaultConfig(context.Background(), creds...)
	if err != nil {
		log.Fatalf("Error loading config from profile: %v", err)
	}
	instanceMap := map[string] *types.Instance{}
	svc := ec2.NewFromConfig(cfg)
	var nextToken *string
	for {
		res, err := svc.DescribeInstances(context.TODO(), &ec2.DescribeInstancesInput{
			DryRun:     false,
			// Not elegant, but eh
			MaxResults: 1000,
			NextToken: nextToken,
		})
		if err != nil {
			log.Fatalf("Could not describe EC2 instances: %v", err)
		}
		for _, r := range res.Reservations {
			for _, i := range r.Instances {
				if i.State.Code != 16 {
					continue
				}
				name := ""
				for _, t := range i.Tags {
					if *t.Key == "Name" {
						name = *t.Value
					}
				}
				instanceMap[fmt.Sprintf("%s %s", *i.InstanceId, name)] = &i
			}
		}
		if res.NextToken == nil {
			break
		}
		nextToken = res.NextToken
	}
	logMsg("Found %d instances", len(instanceMap))
	if len(instanceMap) == 0 {
		log.Fatalf("There were no servers found using the profile \"%s\" and region \"%s\"", *profileF, *regionF)
	}

	chosenInstance, err := getInstance(instanceMap)
	if err != nil {
		log.Fatalf("Error getting user selection: %v", err)
	}
	logMsg("User chose %s\n", *chosenInstance.InstanceId)
	rand.Seed(time.Now().Unix())
	keyPath := path.Join(os.TempDir(), fmt.Sprintf("%d", rand.Int()))
	logMsg("Writing temporary key to path %s", keyPath)
	err = genTmpKey(keyPath)
	if err != nil {
		log.Fatalf("Failed to generate SSH keypair: %v", err)
	}
	pubKeyPath := fmt.Sprintf("%s.pub", keyPath)
	pubKeyFile, err := os.Open(pubKeyPath)
	if err != nil {
		log.Fatalf("Could not open public key %s: %v\n", pubKeyPath, err)
	}
	defer pubKeyFile.Close()
	defer func () {
		logMsg("Deleting private key %s", keyPath)
		os.Remove(keyPath)
	}()
	defer func () {
		logMsg("Deleting public key %s", pubKeyPath)
		os.Remove(pubKeyPath)
	}()
	logMsg("Reading in public key %s", pubKeyPath)
	pubKey, err := ioutil.ReadAll(pubKeyFile)
	logMsg("Sending public key to %s", *chosenInstance.InstanceId)
	eic := ec2instanceconnect.NewFromConfig(cfg)
	spko, err := eic.SendSSHPublicKey(context.TODO(), &ec2instanceconnect.SendSSHPublicKeyInput{
		AvailabilityZone: chosenInstance.Placement.AvailabilityZone,
		InstanceId:       chosenInstance.InstanceId,
		InstanceOSUser:   userF,
		SSHPublicKey:     aws.String(string(pubKey)),
	})
	logMsg("Request ID: %s, Success: %t", *spko.RequestId, spko.Success)
	if err != nil {
		log.Fatalf("Could not send public key to %s: %v", *chosenInstance.InstanceId, err)
	}
	startSsh(*userF, *chosenInstance.PublicIpAddress, keyPath, *portF)
}

func logMsg(msg string, args... interface{}) {
	if *debug {
		log.Printf(msg, args...)
	}
}

func genTmpKey(keyPath string) error {
	cmd := exec.Command("ssh-keygen", "-t", "rsa", "-f", keyPath, "-N", "")
	if *debug {
		cmd.Stderr = os.Stderr
		cmd.Stdout = os.Stdout
	}
	return cmd.Run()
}

func startSsh(user, server, keyPath string, port int) {
	cmd := exec.Command("ssh",
		fmt.Sprintf("%s@%s", user, server),
		"-p", fmt.Sprintf("%d", port),
		"-i", keyPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		panic(err)
	}
}

func getInstance(instances map[string] *types.Instance) (*types.Instance, error) {
	prompt := &survey.Select{
		Message:       "Choose an instance to connect to",
	}
	for k, _ := range instances {
		prompt.Options = append(prompt.Options, k)
	}
	var choice string
	err := survey.AskOne(prompt, &choice)

	return instances[choice], err
}