# Git Access Layer Daemon

_if you are lazy, there is a TL;DR section at the end_

## Introduction

In this document I will try to explain what are our main challenges in scaling GitLab from the Git perspective. It is well known already that our [git access is slow,](https://gitlab.com/gitlab-com/infrastructure/issues/351) and no general purporse solution has been good enough to provide a solid experience. We've also seen than even when using CephFS we can create filesystem hot spots, which implies that pushing the problem to the filesystem layer is not enough, not even with bare metal hardware.

This can be contrasted, among other samples, simply with a look at Rugged::Repository.new performance data, where we can see that our P99 spikes up to 30 wall seconds, while the CPU time keeps in the realm of the 15 milliseconds. Pointing at filesystem access as the culprit.

![rugged.new timings](design/img/rugged-new-timings.png)


Bear in mind that the aim of this effort is to provide first a relief to the main problems that affect GitLab.com availability, then to improve performance, and finally to keep digging down the performance path to reduce filesystem access to it's bare minimum.

## Initial/Current Status of Git Access

Out current storage and git access implementation is as follows

* Each worker as N CephFS or NFS mount points, as many as shards are defined.
* Each worker can take either HTTP or SSH git access.
  * HTTP git access will be handled by workhorse, shelling out to perform the command that was required by the user.
  * SSH git access is handled by ssh itself, with a standard git executable. We only wire authorization in front of it with gitlab-shell.
* Each worker can create Rugged objects or shell out either from unicorn or from sidekiq at it's own discretion.

![current status](design/img/01-current-storage-architecture.png)

One of the main issues we face from production is that this "everyone can do whatever it wants" approach introduces a lot of stress into the filesystem, with the worse result being taking GitLab.com down.

### GitLab.com down when NFS goes down

In this issue we concluded that since we have access to Rugged objects or shelling out, when the filesystem goes down, the whole site goes down. We just don't have a concept of degraded mode at all.

### GitLab.com goes down when it is used as a CDN

* https://gitlab.com/gitlab-com/operations/issues/199
* https://gitlab.com/gitlab-com/infrastructure/issues/506

We have seen this multiple times, the pattern is as follows:

1. Someone distributes a git committed file at GitLab.com and offers access through the API
1. This file is requested by multiple people at roughly the same time (Hackers News effect - AKA slashdot effect)
1. We see increased load in the workers
1. Concidently we see high IO wait
1. We detect hundreds of `git cat-file blob` processes running on the affected workers.
1. GitLab.com is effectively down.

#### Graphs to show at least 2 events like these

##### Event 1

![Workers under heavy load](design/img/git-cat-file-workers.png)

![Decrease on GitLab.com connections](design/img/git-cat-file-connections.png)

##### Event 2

Not necesarily related to git-cat-file-blob, but git was found with a smoking gun

![Workers under heavy load](design/img/git-high-load-workers-load.png)

![Workers with high IOWait](design/img/git-high-load-workers-io-wait.png)

![Drop in GitLab.com Connections](design/img/git-high-load-connections-down.png)

![High git processes count](design/img/git-high-load-process-count.png)

#### What does this mean from the architectural perspective?

These events have 2 possible reads

1. Git can be load and IO intensive, by keeping Git executions in the workers the processes that serve GitLab.com will be competing for these resources, in many cases just loosing the fight, or getting to a locked state if what these processes are trying to do is in fact reach the git storage.
1. Accesses like these create hot spots in the filesystem, we have seen this happening in NFS and in CephFS, more pronounced in CephFS given the latency constraints from the cloud.

![Hot spot architectural design](design/img/02-high-stress-single-point.png)

### GitLab.com git access is slow

I don't think I need to add a lot of data here, it's wildly known and we have plenty data. I'll skip it for now.

## OMG! what can we do?

I think we need to attack all these problems as a whole by isolating and abstracting Git access, first from the worker hosts, then from the application, and then specializing this access layer to provide a fast implementation of the git protocol that does not depends so much in filesystem speed by leveraging memory use for the critical bits.

> It sounds scary and I don't know what to do or how to deal with it!

Let's start with the basics, we need to separate the urgent (availability) from the important (performance)

### Stage one: bulkheads for availability

The first step will be to just remove all the git processes from the workers into a specific fleet of git workers, it's good enough to have just a couple of those as the downside of being attacked will be that these hosts will be under heavy load, not so the workers.

This design will allow the application to fail gracefully when we are being under heavy stress and will allow us to start specializing this git access layer, even including throttling, rate limiting per repo, and monitoring of git usage (something we still don't have) from the minute 0.

The way we will move the process would be by providing a simple client that will simply forward the git command that is being sent either through SSH or HTTPS to a daemon that will be listening on these workers. This daemon will simply spawn a thread (or go routine) where it will execute this git command sending stdin/out to the original client, acting as a transparent proxy for the git access.

The goal here is simply to remove the git execution from the workers, and to build the ground work to keep moving forward.

![Bulkheads architecture](design/img/03-low-stress-single-point.png)

This design will allow us to start walking in the direction of removing git access from the application, but let's keep moving too how it would look like.

### Stage two: specialization for performance

Once we have availability taken care of we will need to start working on the performance side. As I commented previously the main performance hog we are seeing comes from filesystem access as a whole, so that's what we should take care of.

When a Rugged::Repository object is created what happens is that all the refs are loaded in memory so this object can reply to things like _is this a valid repo?_ and _give me the branches names_. For performance reasons these refs could be packed all in one file or they could be spread through multiple files.

Each option has its own benefits and drawbacks. A single file is not nicely managed neither by NFS or CephFS and can create locks contention given enough concurrent access. Multiple files on the other hand translate in multiple file accesses which increases pressure in the filesystem itself, just by opening, reading an closing a lot of tiny files.

I think we need to remove this pressure point as a whole and pull it out of the filesystem completely. It happens that our read to write ration is massive: we read way, way, way more times than we write. A project like gitlab-ce that is under heavy development could have something like a couple of hundred writes, but it will tens of thousands reads per day.

So, I propose that we use this caching layer to load the refs into a memory hashmap. This makes sense because git behaves as a hashmap itself: refs are both keys (branches, tags, the HEAD) and values (the SHA of the git object). Just by caching this in memory we could do the following:

* Serve the refs list from this API, removing calls for 'advertise refs' from git client
* Serve the branches and the tags with the commit ids through and HTTP API that can be consumed by the workers
* Start caching also specifically requested blobs in memory for quick access (to improve the cat-file blob case even further)
* Remove all Rugged::Repository and shell outs from the application by using this API.
* Remove git mountpoints from the application and mount them in this caching layer instead to completely isolate workers from git storage failures.

![High level architecture](design/img/04-git-access-layer-high-level-architecture.png)

> But that sounds like a risky business, how are we going to invalidate the cache? how are we going to control memory usage? how are going to not make it a single point of failure?

Glad you ask!

####



![Pulling Data](design/img/05-git-access-layer-pulling-data.png)


![Pushing Data](design/img/06-git-access-layer-pushing-data.png)

![Final architecture](design/img/07-git-access-layer-final-architecture.png)


## TL;DR:

I think we need to make a fundamental architectural change to how we access git. Both from the application by the use of Rugged or shelling out, or just from the clients themselves running git commands in our infrastructure.

It has proven multiple times that it's easy to take GitLab.com by performing an "attack" by calling git commands on the same repo or blob, generating hotspots which neither CephFS or NFS can survive.

Additionally we have observed that our P99 access time to just create a Rugged object, which is loading and processing the git objects from disk, spikes over 30 seconds, making it basically unusuable. We also saw that just walking through the branches of gitlab-ce requires 2.4 wall seconds. This is clearly unnacceptable.

My idea is not revolutionary. I just think that scaling is specializing, so we need to specialize our git access layer, creating a daemon which initially will only offer removing the git command execution from the workers, then it will focus on building a cache for repo's refs to offer them through an API so we can consume this from the application. Then including hot blob objects to evict CDN type attacks, and finally implementing git upload-pack protocol itself to allow us serving fetches from memory without touching the filesystem at all.
