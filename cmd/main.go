package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/AlecAivazis/survey/v2"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/ec2instanceconnect"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"path"
	"sort"
	"time"
)

var debug *bool

func main() {
	portF := flag.Int("o", 22, "SSH port on the server")
	profileF := flag.String("p", "default", "AWS Profile")
	regionF := flag.String("r", "", "AWS Region")
	userF := flag.String("u", "ec2-user", "Server user")
	debug = flag.Bool("v", false, "Verbose output")
	bastion := flag.Bool("b", false, "Use a bastion server")
	bastionUser := flag.String("bu", "ec2-user", "Bastion username")
	flag.Parse()

	var creds []func(options *config.LoadOptions) error
	if *profileF != "" {
		logMsg("Using profile %s", *profileF)
		creds = append(creds, config.WithSharedConfigProfile(*profileF))
	}
	if *regionF != "" {
		logMsg("Using region %s", *regionF)
		creds = append(creds, config.WithRegion(*regionF))
	}
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
	var bastionInstance *types.Instance
	if *bastion {
		bastionInstance, err = getInstance(instanceMap, true)
		if err != nil {
			log.Fatalf("Could not get bastion")
		}
	}

	chosenInstance, err := getInstance(instanceMap, false)
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
	if *bastion {
		spko, err := eic.SendSSHPublicKey(context.TODO(), &ec2instanceconnect.SendSSHPublicKeyInput{
			AvailabilityZone: bastionInstance.Placement.AvailabilityZone,
			InstanceId:       bastionInstance.InstanceId,
			InstanceOSUser:   userF,
			SSHPublicKey:     aws.String(string(pubKey)),
		})
		logMsg("Request ID: %s, Success: %t", *spko.RequestId, spko.Success)
		if err != nil {
			log.Fatalf("Could not send public key to bastion %s: %v", *chosenInstance.InstanceId, err)
		}
	}
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
	hostName := *chosenInstance.PublicIpAddress
	// if using a bastion, we want to go via a private IP
	if *bastion {
		hostName = *chosenInstance.PrivateIpAddress
	}
	startSsh(*userF, hostName, keyPath, *portF, bastionInstance, *bastionUser)
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

func startSsh(user, server, keyPath string, port int, bastionInstance *types.Instance, bastionUser string) {
	var args []string
	if bastionInstance != nil {
		args = append(args, "-J", fmt.Sprintf("%s@%s", bastionUser, *bastionInstance.PublicIpAddress))
	}
	args = append(args,
		fmt.Sprintf("%s@%s", user, server),
		"-p", fmt.Sprintf("%d", port),
		"-i", keyPath)
	logMsg("ssh %v", args)
	cmd := exec.Command("ssh", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		panic(err)
	}
}

func getInstance(instances map[string]*types.Instance, bastion bool) (*types.Instance, error) {
	msg := "Choose an instance to connect to"
	if bastion {
		msg = "Choose a bastion instance to connect through"
	}
	prompt := &survey.Select{
		Message: msg,
	}
	for k, _ := range instances {
		prompt.Options = append(prompt.Options, k)
	}
	sort.Strings(prompt.Options)
	var choice string
	err := survey.AskOne(prompt, &choice)

	return instances[choice], err
}