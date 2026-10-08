# AI Workflow

This document describes how I used AI tooling to produce this delivery.

Tooling used: GitHub Copilot CLI (model: Kimi K3 & GPT-5.3 codex), interactive sessions in this repo.

## What I delegated to the AI 

In order to achieve this level of project I used AI in many areas, here I distinguish what was done by AI and by human:

By Human:

* The domain layer except for the store, I usually tend to start by the domain representation of the problem to detect early problems on my structures or ambiguities.
* The Decision of using cobra + grpc + grpc-gateway, I think it gives me proven results for proper API creation while avoiding time-consuming issues of doing everything by hand.
* Most of the Makefile as I already had this structure in another project. 
* General ports & adapters structure. I felt was a suitable candidate and I proposed the whole folder structure.
Many things I already have them from other projects, such as the /pkg.

By AI: 

* README.md of the project. All tables and examples to illustrate my work was done by Kimi K3.
* Adapters layer. It was work I knew was going to be easy to handle by copilot
* The proxy, I gave instructions to copilot to have this piece of code in order to make it easy for me to test/debug everything, this way I was able to run the provided CMD and have a log of messages
* Dockerfile to run project in.
* The store file (internal/domain/rockets/store.go).

## What I verified myself 

* Ran and debugged the project locally many times against the provided test binary to check ordering cases, out-of-order, duplicates, gaps, explosion as terminal state.
* Ran the test suite several times for the store, checking proper coverage of the critical paths.


## What I rewrote or rejected from the model's output

* Too many comments auto generated imo. I requested documentation of methods to allow any developer to kick in onto the project easily, but I ended up
redoing many of them. I prefer fewer comments and better readable names.


## What was risky about the agentic workflow and what I would do differently in production

* The store was written without proper tests, and it is a critical piece in order to guarantee proper results, so, in the review process I pointed that to copilot to fix.
* In production, I would rely on proper services for queueing and persistence, obviously.
* Also in general in real projects we usually have bigger context and it is better to use AI with executable tickets instead, I can explain more of this in the defense meeting.

