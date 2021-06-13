# Conch
Conch is a quick and dirty application to get a shell into an EC2 instance with a public IP without having access
to a keypair.

## Building
Run the following to generate a binary in the `build` folder
```shell
$ go build -o build/conch ./...
```

## Running
You will require the following command line utils on the path:
* `ssh`
* `ssh-keygen`

Conch generates a temporary keypair to use in connecting, and just runs the SSH command. Easier
and less error prone than trying to reimplement everything. The simplest usage is just
```shell
$ conch
```

### AWS requirements
You will need to assume a role that is allowed to use the `ec2-instance-connect:SendSSHPublicKey` on
the Ubuntu or Amazon Linux 2 server you want to connect to. 

The server will also need to have 
[the utility](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/ec2-instance-connect-set-up.html#ec2-instance-connect-install)
installed. 

### Arguments
* `-p {AWS Profile name}` - Specify the AWS profile to use
* `-r {AWS Region}` - Specify the AWS region to use
* `-u {EC2 Server instance user}` - Specify the OS user on the EC2 instance
* `-o {SSH port}` - Specify the SSH port
* `-v` - Verbose logging
* `-b` - Use a bastion server
* `-bu` - Bastion username, default `ec2-user`