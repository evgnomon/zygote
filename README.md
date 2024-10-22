<p align="center">
<img src="docs/assets/Zygote.svg" width="256" height="256">
</p>
<p align="center"> Zygote </p>
<p align="center"> A serverless function runtime that you choose who has custody of the servers! </p>


> Secure your exit right now as "no human is let in the factory" in 44 minutes, as the factory can print everything including itself!

> Dec 2023

You describe infrastructure requirements is a `.zygote.toml` file and put it close to your function's source code and check-in into your function's git repo, and you don't care about any thing else! Zygote can quickly fork new environments tracking the config file(s) in the `head` of your Git repo. So you don't only branch function source code but also its environment which is needed to run the function for testing and staging. And all this is transparently handled by Zygote.

You can install `zygote` binary on your local machine to setup a real local environment (not a mock) for development and testing. Linux, macOS and Windows (Both native and over WSL2) are supported for local development. The same way you can test your cloud functions in CI by running an instance of Zygote in your CI environment. Bultin support for MySQL InnoDB cluster as the general purpose Table store and a sharded Redis cluster as In-Memory KeyValue store and streaming fabric comes built in with Zygote. You can use these services in your functions without any extra setup. You can also use any other service you like, Zygote is designed to be extensible and you can add your own services to it.

Zygote is a server-less runtime that you choose who custody the servers! You have two options using Zygote in production, subscribe to Zygote .Run and run your cloud function in there. Or Install everything on your own machine(s). Zygote is open sourced for you and it is production ready, just fork it and start using it.
