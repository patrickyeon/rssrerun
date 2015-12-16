package rssmangle

import (
    "io"
    "fmt"
    "encoding/xml"
)

type Node struct {
    Name xml.Name
    Attrs []xml.Attr
    Content []byte
    Children []*Node
    Tag string
}

type Feed struct {
    Root *Node
    Items []*Node
}

func NewNode(t xml.Token) *Node {
    n := new(Node)
    switch t.(type) {
    case xml.StartElement:
        tt := t.(xml.StartElement)
        n.Name = tt.Name
        n.Attrs = tt.Attr
    case xml.Comment:
        n.Tag = "comment"
        // does not include <!-- -->
        n.Content = t.(xml.Comment).Copy()
    case xml.ProcInst:
        // <?target inst?>
        tt := t.(xml.ProcInst).Copy()
        n.Tag = "procinst"
        n.Attrs = []xml.Attr{{xml.Name{"","Target"}, tt.Target}}
        n.Content = tt.Inst
    case xml.Directive:
        n.Tag = "directive"
        // does not include <! >
        n.Content = t.(xml.Directive).Copy()
    case xml.CharData:
        n.Tag = "chardata"
        n.Content = t.(xml.CharData).Copy()
    }
    return n
}

func NewFeed(t io.Reader) *Feed {
    dec := xml.NewDecoder(t)
    var root *Node
    var stack []*Node
    var preTokens []*Node
    var postTokens []*Node
    for t, _ := dec.Token(); t != nil; t, _ = dec.Token(){
        switch t.(type) {
        case xml.StartElement:
            n := NewNode(t.(xml.StartElement))
            if len(stack) == 0 {
                if root != nil {
                    fmt.Println("more than one root")
                    return nil
                }
                root = n
            } else {
                parent := stack[len(stack) - 1]
                parent.Children = append(parent.Children, n)
            }
            stack = append(stack, n)
        case xml.EndElement:
            if stack[len(stack) - 1].Name != t.(xml.EndElement).Name {
                fmt.Println("not closed:", stack[len(stack) - 1].Name)
            } else {
                stack = stack[:len(stack) - 1]
            }
        case xml.CharData:
            if len(stack) == 0 {
                if root == nil {
                    preTokens = append(preTokens, NewNode(t))
                } else {
                    postTokens = append(postTokens, NewNode(t))
                }
            } else {
                stack[len(stack) - 1].Content = t.(xml.CharData).Copy()
            }
        default:
            if len(stack) == 0 {
                if root == nil {
                    preTokens = append(preTokens, NewNode(t))
                } else {
                    postTokens = append(postTokens, NewNode(t))
                }
            } else {
                parent := stack[len(stack) - 1]
                parent.Children = append(parent.Children, NewNode(t))
            }
        }
    }
    f := new(Feed)
    f.Root = root
    for stack = []*Node{root}; len(stack) > 0; stack = stack[1:] {
        node := stack[0]
        if node.Name.Local == "channel" {
            for _, c := range node.Children {
                if c.Name.Local == "item" {
                    f.Items = append(f.Items, c)
                }
            }
            break
        } else {
            for _, c := range node.Children {
                stack = append(stack, c)
            }
        }
    }
    return f
}
