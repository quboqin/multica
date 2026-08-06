# Market Pack Component Extraction Source Map

| Contract | Source |
| --- | --- |
| Creating an extraction freezes one source attachment and dispatches the configured capability Agent | `server/internal/handler/creative_market_pack_extraction.go:CreateCreativeMarketPackComponentExtraction`, `server/internal/handler/creative_market_pack_extraction.go:resolveMarketPackComponentExtractionAgent` |
| `multica creative market-pack component-extraction get` returns the frozen extraction and source coordinates | `server/cmd/multica/cmd_creative_domain.go:runCreativeMarketPackExtractionGet`, `server/internal/handler/creative_market_pack_extraction.go:GetCreativeMarketPackComponentExtraction` |
| `multica attachment download` retrieves the authenticated source attachment | `server/cmd/multica/cmd_attachment.go:runAttachmentDownload` |
| `multica creative market-pack component-extraction put` saves candidate rectangles without publishing or applying them | `server/cmd/multica/cmd_creative_domain.go:runCreativeMarketPackExtractionPut`, `server/internal/handler/creative_market_pack_extraction.go:PutCreativeMarketPackComponentExtraction` |
| Applying confirmed candidates is a separate human API action | `server/internal/handler/creative_market_pack_extraction.go:ApplyCreativeMarketPackComponentExtraction` |
