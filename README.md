# Zygote: Cloud Functions Runtime for Factories without Human!

> In a world where "no human is let in the factory", machine is nothing than a domain name exposing a REST API (or just a nice UI perhaps an AI voice). I subscribe to that machine to get accelerated. And eventually there would be no job left except integrating the next block of code to that world wide machine. There is only 44 minutes left to have the machine terminate the human in the factory! The only thing I can hopefully do is to secure the way out particularly for myself and perhaps for my family. An escape out in a most efficient and accurate way without leaving myself inside another factory! I build a digital spaceship to secure the way out and share its parts with you so you can also secure the way out for yourself and your family and also for me perhaps! Instead of slowing it down, let's get accelerated toward "no human is let in the factory" before it is too late!

> Hamed Ghasemzadeh 2023

Zygote is an integrated runtime for developing cloud functions guided by the principle "No human is let in the factory". Zygote building blocks are open sourced under HGL license for free to the extend which you can develop and test a cloud function on your local machine. The function code gets seamlessly integrated using Zygote .Run CI/CD and serverless cloud which is offered as a service and could be added to your Github repo as an Application. This ensures you can get a function up and running "without entering the factory". Zygote tool and the runtime is freely available under HGL License and could be quickly setup locally on your machine supporting Linux, macOS and Windows (Both native and WSL2).

Zygote has bultin support for MySQL InnoDB cluster as the default database and sharded Redis cluster as in-memory store and messaging service which makes it self sufficient out of the box to quickly create the first scaleable HTTP function which both works locally, in the cloud and on-prem so gives the choice to you choosing the factory without entering it.

You can seamlessly integrate functions by subscribing to Zygote .Run serverless cloud offering, leveraging integrated open-source/free databases and additional open services. This setup allows for offline development with full access to building blocks, offering the flexibility to modify the source code of these components if needed. Additionally, our architecture supports transitioning away from our serverless cloud, enabling you to host these services on your own platform if desired. Thus, the serverless cloud subscription is a booster to provide a progressive experience, beginning with local development and offering the option to migrate to your own data center, should you choose to move away from our serverless solutions when "no human is let in the factory" anymore.

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
