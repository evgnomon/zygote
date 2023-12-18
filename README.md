# Zygote: Cloud Functions Runtime for Factories without Human!

> In a world where "no human is let in the factory", machine is nothing than a domain name exposing a REST API (or equivalently a nice UI perhaps an AI voice) with having me as a paying subscriber. And eventually there would be no job left except integrating the next block of code to some code repository which is the only gateway to modify that digital world. There is only 44 minutes left. Fortunately, The only thing I can hopefully do is to secure the way out in a most efficient and accurate way for my family to escape out the factory when "no human is let in the factory" anymore! My idea is to build a digital spaceship to secure the way out and share it with you so you can also secure the way out for yourself and your family! So let's build for "no human is let in the factory" before it is too late!

> Hamed Ghasemzadeh 2023

Zygote is a handy runtime for developing cloud functions guided by the principle "No human is let in the factory". The function code gets seamlessly integrated using Zygote CI/CD which is offered as a service and could be added to your Github repo as an Application. This ensures you can get a function up and running "without entering the factory". Zygote tool and the runtime is freely available under HGL License and could be quickly setup locally on your machine supporting Linux, macOS and Windows (Both native and WSL2).

Zygote has bultin support for MySQL InnoDB cluster as the default database and sharded Redis cluster as in-memory store and messaging service which makes it self sufficient out of the box to quickly create the first scaleable HTTP function which both works locally, in the cloud and on-prem so gives the choice to you choosing the factory without entering it.

You can seamlessly integrate functions by subscribing to Zygote.run serverless cloud offering, leveraging integrated open-source/free databases and additional open services. This setup allows for offline development with full access to building blocks, offering the flexibility to modify the source code of these components if needed. Additionally, our architecture supports transitioning away from our serverless cloud, enabling you to host these services on your own platform if desired. Thus, the serverless cloud subscription is a booster to provide a progressive experience, beginning with local development and offering the option to migrate to your own data center, should you choose to move away from our serverless solutions when "no human is let in the factory" anymore.

## Quick Start Guide
```bash
git clone git@github.com:evgnomon/zygote.git
go install .

cd examples/example-project-js

zygote add -z mysql:[VM1,VM2,VM3] -z app:[VM4] # Run app on a clustered mysql instance on port 80
zygote stop # Stop everything
zygote restart # Rest everything
zygote restart -z mysql:[VM1] # Rest VM1 while the cluster is operating
zygote restart -z app:[VM4,VM5] # Add new VM5 to the set
zygote restart -z mysql # Restart the mysql cluster
zygote replace -z mysql:[VM1]:[VM6] # Replace VM1 with VM6 for mysql cluster so the cluster will be [VM6,VM2,VM3] afterward
zygote add -z redis:[VM7,VM8,VM9] # Add a new redis instance to the running cluster
zygote rename -z redis:redis-1 # Rename redis to redis-1 keeping everything else
zygote add -z redis-2:[VM10,VM11,VM12] # Add a new redis shard to the running cluster
zygote fork -e stage # Fork everything to the stage environment (from the default env.) VM1-stage, VM2-stage would be machine names.
zygote switch -e stage # commands afterward target stage
zygote fork -e prod # create prod environment
zygote switch -e prod # switch to prod
zygote mount -z app --url www.example.app:80/myapp
zygote forget # Bye, nothing exists now!
```
VMs are defined in ~/.ssh/config
