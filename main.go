package main

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"os/exec"
	"strings"

	"github.com/fatih/color"
	"github.com/xlab/treeprint"
)

func git(arg ...string) ([]string, error) {
	buf := &bytes.Buffer{}
	cmd := exec.Command("git", arg...)
	cmd.Stdout = buf
	if err := cmd.Run(); err != nil {
		return nil, err
	}

	lines := []string{}
	scanner := bufio.NewScanner(buf)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return lines, nil
}

type Branch struct {
	Name    string
	Current bool
}

func listBranches() ([]Branch, error) {
	lines, err := git("branch")
	if err != nil {
		return nil, err
	}

	branches := make([]Branch, len(lines))
	for i, line := range lines {
		if line[0] == '*' {
			branches[i].Current = true
		}
		branches[i].Name = line[2:]
	}

	return branches, nil
}

type Commit struct {
	Hash     string
	Subject  string
	Author   string
	OnMaster bool
}

func listCommits(branch string) ([]Commit, error) {
	lines, err := git("log", "--pretty=format:%H#%an#%s", "--max-count=1000", branch)
	if err != nil {
		return nil, err
	}

	commits := make([]Commit, len(lines))
	for i, line := range lines {
		parts := strings.SplitN(line, "#", 3)
		commits[i].Hash = parts[0]
		commits[i].Author = parts[1]
		commits[i].Subject = parts[2]
	}
	return commits, nil
}

type CommitNode struct {
	Commit
	Branches []Branch
	Children []*CommitNode
}

func (cn CommitNode) ToTree(tree treeprint.Tree) {
	var childBranch treeprint.Tree
	data := cn.Subject
	if cn.Hash != "" {
		data = fmt.Sprintf("%s (%s, %s)", cn.Subject, cn.Author, cn.Hash[:8])
	}
	if len(cn.Branches) > 0 {
		meta := ""
		for i, branch := range cn.Branches {
			if i != 0 {
				meta += ", "
			}
			if branch.Current {
				meta += color.New(color.FgGreen).Sprint(branch.Name)
			} else {
				meta += branch.Name
			}
		}
		childBranch = tree.AddMetaBranch(meta, data)
	} else {
		childBranch = tree.AddBranch(data)
	}
	for i, child := range cn.Children {
		if i == 0 && (len(cn.Branches) == 0 || cn.OnMaster) {
			child.ToTree(tree)
		} else if i == len(cn.Children)-1 {
			child.ToTree(childBranch)
		} else {
			newBranch := childBranch.AddBranch("┐")
			child.ToTree(newBranch)
		}
	}
}

func (cn CommitNode) String() string {
	result := cn.Subject
	if len(cn.Children) > 0 {
		result += "("
		for _, child := range cn.Children {
			result += "\n"
			result += child.String()
		}
		result += "\n)"
	}
	return result
}

func main() {
	branches, err := listBranches()
	if err != nil {
		log.Fatal(err)
	}

	mCommits, err := listCommits("master")
	if err != nil {
		log.Fatal(err)
	}

	mMap := make(map[string]Commit)
	for _, commit := range mCommits {
		mMap[commit.Hash] = commit
	}

	var masterBranch Branch
	cns := make(map[string]*CommitNode)
	mNeeded := make(map[string]bool)
	var ok bool
	var cn, lastCn *CommitNode
	for _, branch := range branches {
		if branch.Name == "master" {
			masterBranch = branch
			continue
		}
		lastCn = nil

		commits, err := listCommits(branch.Name)
		if err != nil {
			log.Fatal(err)
		}

		for i, commit := range commits {
			if cn, ok = cns[commit.Hash]; !ok {
				cn = &CommitNode{
					Commit: commit,
				}
				cns[commit.Hash] = cn
			}
			if i == 0 {
				cn.Branches = append(cn.Branches, branch)
			}
			if lastCn != nil {
				cn.Children = append(cn.Children, lastCn)
			}
			lastCn = cn

			if ok {
				break
			}
			if _, ok := mMap[commit.Hash]; ok {
				mNeeded[commit.Hash] = true
				break
			}
		}
	}

	lastCn = nil
	for i, commit := range mCommits {
		if cn, ok = cns[commit.Hash]; !ok {
			cn = &CommitNode{
				Commit: commit,
			}
			cns[commit.Hash] = cn
		}
		cn.OnMaster = true
		if i == 0 {
			cn.Branches = append(cn.Branches, masterBranch)
		}
		if lastCn != nil && !hasChild(cn, lastCn.Hash) {
			cn.Children = append([]*CommitNode{lastCn}, cn.Children...)
		}
		_, inNeeded := mNeeded[commit.Hash]
		if inNeeded || i == 0 {
			lastCn = cn
		} else if lastCn.Subject != "..." {
			cn.Subject = "..."
			cn.Hash = ""
			lastCn = cn
		}

		delete(mNeeded, commit.Hash)
		if len(mNeeded) == 0 {
			break
		}
	}
	tree := treeprint.New()
	cn.ToTree(tree)
	fmt.Println(tree.String())
}

func hasChild(node *CommitNode, hash string) bool {
	for _, child := range node.Children {
		if child.Hash == hash {
			return true
		}
	}
	return false
}
