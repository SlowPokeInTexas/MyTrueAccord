# Sean Hoffman True Accord Coding test

The github for this project is:
https://github.com/SlowPokeInTexas/MyTrueAccord/


## Building

This code was built using go modules. As indicated in the .mod file, this was built with go 1.16.4
1. Extract the directory
2. cd into the directory
3. Build using "go build"


## Running
Linux and Mac:
1. cd into the extracted directory
2. ./true-accord

Windows:
1. cd into the extracted directory
2. true-accord

Output will go to stdio

## Design and Assumptions
Having worked on mark-to-market and interest calculating applications past roles,
not counting payments that occurred within a couple days of a scheduled payment
date really bugged me, and in my original design I started to account for a
grace period for a payment. I realized though after re-reading the spec
that it would have put my solution out-of-spec.

Halfway through the process of calculating the next payment date, I realized
that in an actual application there would be use for keeping/seeing/viewing
a payment schedule. I added those to the internal structure (but not to the
JSON output).

Other Assumptions:
- I wrote the retrieval of the data from the external service as go-routines so they could be executed in parallel

- I decided to take the output from the 3 separate web-service calls and put them into an object graph to make it
  more "object oriented" like and easier to manipulate

- I made a conscious decision to make the URIs that are in use const values in the code to protect from evil/nefarious
  modifications
