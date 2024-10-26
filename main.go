package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/g8rswimmer/go-twitter/v2"
	"github.com/gorilla/websocket"
	"github.com/spf13/cobra"
	log "k8s.io/klog"
)

type authorize struct {
	Token string
}

func (a authorize) Add(req *http.Request) {
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", a.Token))
}

func GetUserInfo(c *twitter.Client, name string) (*twitter.UserObj, error) {
	opts := twitter.UserLookupOpts{
		UserFields: []twitter.UserField{
			twitter.UserFieldCreatedAt,
			twitter.UserFieldDescription,
			twitter.UserFieldEntities,
			twitter.UserFieldID,
			twitter.UserFieldLocation,
			twitter.UserFieldName,
			twitter.UserFieldPinnedTweetID,
			twitter.UserFieldProfileImageURL,
			twitter.UserFieldProtected,
			twitter.UserFieldPublicMetrics,
			twitter.UserFieldURL,
			twitter.UserFieldUserName,
			twitter.UserFieldVerified,
			twitter.UserFieldWithHeld,
		},
		Expansions: []twitter.Expansion{twitter.ExpansionPinnedTweetID},
	}

	userResponse, err := c.UserNameLookup(context.Background(), strings.Split(name, ","), opts)

	if err != nil {
		return nil, err
	}

	return userResponse.Raw.Users[0], err
}

func UserTimeline(c *twitter.Client, userId string, dig bool) (tweets []*twitter.MediaObj, err error) {
	opts := twitter.UserTweetTimelineOpts{
		TweetFields: []twitter.TweetField{
			twitter.TweetFieldID,
			twitter.TweetFieldText,
			twitter.TweetFieldAttachments,
			twitter.TweetFieldAuthorID,
			twitter.TweetFieldContextAnnotations,
			twitter.TweetFieldConversationID,
			twitter.TweetFieldCreatedAt,
			twitter.TweetFieldEntities,
			twitter.TweetFieldGeo,
			twitter.TweetFieldInReplyToUserID,
			twitter.TweetFieldLanguage,
			twitter.TweetFieldPublicMetrics,
			//twitter.TweetFieldNonPublicMetrics,
			//twitter.TweetFieldOrganicMetrics,
			//twitter.TweetFieldPromotedMetrics,
			twitter.TweetFieldPossiblySensitve,
			twitter.TweetFieldReferencedTweets,
			twitter.TweetFieldSource,
			twitter.TweetFieldWithHeld,
		},
		UserFields: []twitter.UserField{twitter.UserFieldUserName},
		Expansions: []twitter.Expansion{twitter.ExpansionAttachmentsMediaKeys},
		MediaFields: []twitter.MediaField{
			twitter.MediaFieldDurationMS,
			twitter.MediaFieldHeight,
			twitter.MediaFieldMediaKey,
			twitter.MediaFieldPreviewImageURL,
			twitter.MediaFieldType,
			twitter.MediaFieldURL,
			twitter.MediaFieldWidth,
			twitter.MediaFieldPublicMetrics,
			//twitter.MediaFieldNonPublicMetrics,
			//twitter.MediaFieldOrganicMetrics,
			//twitter.MediaFieldPromotedMetrics,
			twitter.MediaFieldAltText,
			twitter.MediaFieldVariants,
		},
		MaxResults: 100,
		Excludes:   []twitter.Exclude{twitter.ExcludeRetweets, twitter.ExcludeReplies},
	}
	likesFilter := 100
	var mediaObjs []*twitter.MediaObj
	for {
		timeline, err := c.UserTweetTimeline(context.Background(), userId, opts)
		log.Errorln("try UserTweetTimeline...")

		if err != nil {
			log.Errorln(err)
			break
		}
		log.Errorln("raw %v", timeline.Meta)

		//mediaObjs = append(mediaObjs, timeline.Raw.Includes.Media...)

		// Loop through each tweet to check likes and append relevant media
		for _, tweet := range timeline.Raw.Tweets {
			if tweet.PublicMetrics != nil && tweet.PublicMetrics.Likes >= likesFilter {
				// If the tweet has attachments and likes are greater or equal to likesFilter
				if tweet.Attachments != nil {
					for _, mediaKey := range tweet.Attachments.MediaKeys {
						if media, ok := timeline.Raw.Includes.MediaByKeys()[mediaKey]; ok {
							mediaObjs = append(mediaObjs, media)
						}
					}
				}
			}
		}

		if timeline.Meta.NextToken == "" {
			break
		}
		opts.PaginationToken = timeline.Meta.NextToken
		if !dig {
			break
		}
	}

	return mediaObjs, nil
}

func main() {
	var rootCmd = &cobra.Command{Use: "x-cli"}

	var cmdHello = &cobra.Command{
		Use:   "hello",
		Short: "Prints Hello, world!",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Hello, world!")
		},
	}

	var cmdDownloadMedia = &cobra.Command{
		Use:   "fk [name]",
		Short: "Download media for a given user name",
		Args:  cobra.ExactArgs(1), // 确保只接受一个参数
		Run:   downloadMedia,
	}

	rootCmd.AddCommand(cmdHello)
	rootCmd.AddCommand(cmdDownloadMedia)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func downloadMedia(cmd *cobra.Command, args []string) {
	name := args[0]

	// 创建以name命名的文件夹
	folderPath := "./" + name
	//if err := os.MkdirAll(folderPath, os.ModePerm); err != nil {
	//	fmt.Printf("Error creating directory: %v\n", err)
	//	return
	//}

	client := &twitter.Client{
		Authorizer: authorize{
			Token: "",
		},
		Client: http.DefaultClient,
		Host:   "https://api.twitter.com",
	}

	// 使用提供的client和name获取用户信息和媒体对象
	user, err := GetUserInfo(client, name)
	if err != nil {
		fmt.Printf("Error getting user info: %v\n", err)
		return
	}
	fmt.Printf("Getting user info: %v %v\n", user.ID, user.Description)

	mediaObjs, err := UserTimeline(client, user.ID, true)
	if err != nil {
		fmt.Printf("Error getting user timeline: %v\n", err)
		return
	}
	fmt.Printf("Getting user timeline: %v\n", len(mediaObjs))

	// 遍历mediaObjs并下载
	for _, mediaObj := range mediaObjs {
		fmt.Printf("Downloading %s...\n", mediaObj.URL)
		if mediaObj.Type == "video" {
			maxRate := 0
			for _, v := range mediaObj.Variants {
				if v.BitRate > maxRate {
					maxRate = v.BitRate
					mediaObj.URL = v.URL
				}
			}
		}
		err = downloadFile(folderPath, mediaObj.URL+":orig", mediaObj.Key, "p3terx")
		if err != nil {
			fmt.Printf("Error downloading file: %v\n", err)
		}
	}
	fmt.Println("Download completed.")
}

// 调用aria2的JSON-RPC接口下载文件（通过WebSocket）
func downloadFile(folderPath, u, fileName, secret string) error {
	// 构造WebSocket连接
	dialer := websocket.Dialer{}
	conn, _, err := dialer.Dial("ws://192.168.151.62:6800/jsonrpc", nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	re := regexp.MustCompile(`\.(mp4|jpg)(\?[^?]*|:[^:]*|$)$`)
	extMatches := re.FindStringSubmatch(u)
	if len(extMatches) > 1 {
		ext := "." + extMatches[1]
		fileName += ext
	}

	// 构造JSON-RPC请求体
	requestBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "aria2.addUri",
		"id":      "qwer",
		"params": []interface{}{
			"token:" + secret, // 使用RPC密钥
			[]string{u},
			map[string]string{
				"dir":             "/downloads/" + folderPath,
				"out":             fileName,
				"allow-overwrite": "true",
			},
		},
	}

	// 序列化JSON请求体
	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return err
	}

	// 发送数据到WebSocket服务
	err = conn.WriteMessage(websocket.TextMessage, jsonData)
	if err != nil {
		return err
	}

	// 接收响应（可选）
	_, message, err := conn.ReadMessage()
	if err != nil {
		return err
	}
	fmt.Printf("Received: %s\n", message)

	return nil
}
