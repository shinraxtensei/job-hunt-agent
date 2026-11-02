// This file only fences web/ off from the root Go module, so `go ... ./...` never walks
// web/node_modules (some npm packages ship .go files). There is no Go code here.
module github.com/shinraxtensei/job-hunter/web

go 1.27
