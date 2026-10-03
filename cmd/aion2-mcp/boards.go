package main

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	aion2 "github.com/nuriland/aion2-api"
)

func boardTools(s *mcp.Server, cs *clients) {
	add(s, cs, "list_posts", "A board's posts, newest first, without their bodies", listPosts)
	add(s, cs, "list_pinned_posts", "The posts NC pinned above a board", listPinnedPosts)
	add(s, cs, "get_post", "One post with its body", getPost)
	add(s, cs, "list_comments", "The comments under a post, newest first, each followed by its replies. NC serves them all, limit only trims the reply", listComments)
}

type boardArgs struct {
	scope

	Board aion2.Board `json:"board"`
}

type postsArgs struct {
	boardArgs

	Limit listLimit `json:"limit,omitempty" jsonschema:"at most this many, newest first (default 20)"`
}

type postArgs struct {
	boardArgs

	ID     string     `json:"id" jsonschema:"the post id from list_posts"`
	Format postFormat `json:"format,omitempty" jsonschema:"text (default) strips the body's markup, html keeps its html"`
}

type commentsArgs struct {
	boardArgs

	PostID string    `json:"postId" jsonschema:"the post id from list_posts or get_post"`
	Limit  listLimit `json:"limit,omitempty" jsonschema:"at most this many, newest first (default 20)"`
}

// postText is a post with its body flattened to text in place of the HTML
type postText struct {
	*aion2.Post

	Text string `json:"text"`
}

// listPosts stops paging the board once it has the limit
func listPosts(ctx context.Context, c aion2.Aion2Client, args postsArgs) (list[aion2.Post], error) {
	var (
		out list[aion2.Post]
		n   = args.Limit.or(20)
	)
	for post, err := range c.Posts(ctx, args.Board) {
		if err != nil {
			return out, err
		}

		if len(out.Items) == n {
			out.Truncated = true
			break
		}
		out.Items = append(out.Items, post)
	}
	return out, nil
}

func listPinnedPosts(ctx context.Context, c aion2.Aion2Client, args boardArgs) (list[aion2.Post], error) {
	posts, err := c.PinnedPosts(ctx, args.Board)
	return list[aion2.Post]{Items: posts}, err
}

// getPost answers html with the post as NC sends it, and text, the default, with a postText
func getPost(ctx context.Context, c aion2.Aion2Client, args postArgs) (any, error) {
	post, err := c.Post(ctx, args.Board, args.ID)
	if err != nil || args.Format == "html" {
		return post, err
	}

	text := htmlText(post.HTML)
	post.HTML = ""
	return postText{post, text}, nil
}

func listComments(ctx context.Context, c aion2.Aion2Client, args commentsArgs) (list[aion2.Comment], error) {
	comments, err := c.Comments(ctx, args.Board, args.PostID)
	return truncate(comments, args.Limit), err
}
