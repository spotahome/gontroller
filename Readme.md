# Gontroller

Controllers and operators without Kubernetes.

## Introduction

Kubernetes controllers/operators are based on controller pattern a.k.a reconciliation loop, this pattern is based on maintaining the desired state and self-heal if it is required. It's a very resilient and robust pattern to maintain and automate tasks.

Gontroller let's you apply this pattern on non-kubernetes applications. You don't need to use a Kubernetes apiserver to subscribe for events, CRDs... so you can create an operator, you can use simple structs as objects and you will only need to implement a few interfaces to use this pattern.

## Features

## How does it work

## Why not use a simple loop?
