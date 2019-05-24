package light

const (
	//VoteAgreeAgainst vote sign with against
	VoteAgreeAgainst = iota
	//VoteAgree vote sign with agree
	VoteAgree

	Normal = iota
	DiscTooManyPeers
	FetcherCall
	DownloaderCall
	SFetcherCall
	SDownloaderCall

	PeerSendCall
	FetcherHeadCall
	SFetcherHeadCall
	DownloaderFetchCall
	DownloaderPartCall
	SDownloaderFetchCall
	SDownloaderLoopCall
	SDownloaderPartCall
)
