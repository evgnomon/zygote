# Zygote: Cloud Function Runtime for Factories without Human!

> In a world where "no human is let in the factory", machine is a domain name for IO. I subscribe to that machine to get accelerated which gives acceleration to the machine too. There would be no other place left for me except my personal lab! There is only 44 minutes left! In a digital world there can be only one main branch of code eventually integrating everything which always points to the latest working code block. And a digital username and one speaking under that digital username with human cells based on zygote so we know which block of code gets integrated next to the main ledger of everything. Yes, This is the only direction left and that is why it is the final stage too.

> evgnomon Dec 2023

Zygote is an integrated runtime for developing cloud functions guided by the principle "No human is let in the factory". Zygote building blocks are open sourced under HGL license for free to the extend which you can develop and test a cloud function on your local machine offline (which we call it personal lab). The function code gets seamlessly integrated using Zygote .Run (Zygote dot Run) CI/CD and serverless cloud which is offered as a service and could be added to your Github repo as an Application. This ensures you can get a function up and running "without entering the factory". Zygote tool and the runtime is freely available under HGL License and could be quickly setup locally on your machine supporting Linux, macOS and Windows (Both native and WSL2).

Zygote .Run subscription only accepts Bitcoin (Lightning) as payment method simply because we assume there is no Zygote .Run user unless they already have some Bitcoin secured (isn't it?). So Bitcoin is both money and the security for Zygote!

Zygote has bultin support for MySQL InnoDB cluster as the default database and sharded Redis cluster as in-memory store and messaging service which makes it self sufficient out of the box to quickly create the first scaleable HTTP function which both works locally, in the cloud and on-prem so gives the choice to you choosing the factory without entering it (and also helps us to not get stuck into our own factory).

You can seamlessly integrate functions by subscribing to Zygote .Run serverless cloud offering, leveraging integrated open-source/free databases and additional open services up and running at scale. This setup allows for offline development with open access to building blocks (licensed under HGL, GPL etc). Additionally, our architecture supports transitioning away from our serverless cloud, enabling you to host these services on your own platform if desired. Thus, the serverless cloud subscription is a booster to provide a progressive experience, beginning with local development and offering the option to migrate to your own data center, should you choose to move away from our serverless solutions when "no human is let in the factory" anymore.

## Quick Start Guide
```bash
git clone git@github.com:evgnomon/zygote.git
go install .

cd examples/example-project-js

zygote add -z mysql:[1,2,3] -z app:[1] # Run app on a clustered mysql instance on port 80
zygote stop # Stop everything
zygote restart # Rest everything
zygote restart -z mysql:[1] # Rest [1] while the cluster is operating
zygote restart -z app:[4,5] # Add new VM5 to the set
zygote restart -z mysql # Restart the mysql cluster
zygote replace -z mysql:[1]:[6] # Replace VM1 with VM6 for mysql cluster so the cluster will be [6,2,3] afterward
zygote add -z redis:[7,8,9] # Add a new redis instance to the running cluster
zygote rename -z redis:redis-1 # Rename redis to redis-1 keeping everything else
zygote add -z redis-2:[10,11,12] # Add a new redis shard to the running cluster
zygote fork -e stage # Fork everything to the stage environment (from the default env.) 1-stage, 2-stage would be machine names.
zygote switch -e stage # commands afterward target stage
zygote fork -e prod # create prod environment
zygote switch -e prod # switch to prod
zygote mount -z app --url www.example.app:80/myapp
zygote forget # Bye, nothing exists now!
```
VMs are defined in ~/.ssh/config
