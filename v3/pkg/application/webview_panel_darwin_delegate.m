//go:build darwin && !ios && !server

#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>

extern void processPanelURLRequest(unsigned int, void *, bool);
extern void cancelURLRequest(void *);
extern void panelDidCommitURL(unsigned int, unsigned int, const char *);

@interface WailsPanelSchemeHandler : NSObject <WKURLSchemeHandler>
@property unsigned int windowId;
@end

@implementation WailsPanelSchemeHandler
- (void)webView:(WKWebView *)webView startURLSchemeTask:(id<WKURLSchemeTask>)urlSchemeTask {
    NSURLRequest *request = urlSchemeTask.request;
    BOOL ownPage = [[webView.URL.scheme lowercaseString] isEqualToString:@"wails"];
    BOOL mainDocument = request.mainDocumentURL != nil && [request.URL isEqual:request.mainDocumentURL];
    processPanelURLRequest(self.windowId, urlSchemeTask, !(ownPage || mainDocument));
}
- (void)webView:(WKWebView *)webView stopURLSchemeTask:(id<WKURLSchemeTask>)urlSchemeTask {
    cancelURLRequest(urlSchemeTask);
    NSInputStream *stream = urlSchemeTask.request.HTTPBodyStream;
    if (stream) {
        NSStreamStatus status = stream.streamStatus;
        if (status != NSStreamStatusClosed && status != NSStreamStatusNotOpen) {
            [stream close];
        }
    }
}
@end

@interface WailsPanelDelegate : NSObject <WKNavigationDelegate, WKUIDelegate>
@property unsigned int windowId;
@property unsigned int panelId;
@end

@implementation WailsPanelDelegate
- (WKWebView *)webView:(WKWebView *)webView createWebViewWithConfiguration:(WKWebViewConfiguration *)configuration
    forNavigationAction:(WKNavigationAction *)navigationAction windowFeatures:(WKWindowFeatures *)windowFeatures {
    NSURL *url = navigationAction.request.URL;
    NSString *scheme = [url.scheme lowercaseString];
    if ([scheme isEqualToString:@"https"] || [scheme isEqualToString:@"http"] || [scheme isEqualToString:@"mailto"]) {
        [[NSWorkspace sharedWorkspace] openURL:url];
    }
    return nil;
}
- (void)webView:(WKWebView *)webView decidePolicyForNavigationAction:(WKNavigationAction *)navigationAction
    decisionHandler:(void (^)(WKNavigationActionPolicy))decisionHandler {
    // A file dropped on the view would otherwise replace the page.
    if (navigationAction.request.URL.isFileURL) {
        decisionHandler(WKNavigationActionPolicyCancel);
        return;
    }
    decisionHandler(WKNavigationActionPolicyAllow);
}
- (void)webView:(WKWebView *)webView didCommitNavigation:(WKNavigation *)navigation {
    NSString *url = webView.URL.absoluteString;
    if (url != nil) {
        panelDidCommitURL(self.windowId, self.panelId, url.UTF8String);
    }
}
- (void)webViewWebContentProcessDidTerminate:(WKWebView *)webView {
    [webView reload];
}
- (void)webView:(WKWebView *)webView runOpenPanelWithParameters:(WKOpenPanelParameters *)parameters
    initiatedByFrame:(WKFrameInfo *)frame completionHandler:(void (^)(NSArray<NSURL *> *URLs))completionHandler {
    NSOpenPanel *openPanel = [NSOpenPanel openPanel];
    openPanel.allowsMultipleSelection = parameters.allowsMultipleSelection;
    if (@available(macOS 10.14, *)) {
        openPanel.canChooseDirectories = parameters.allowsDirectories;
    }
    [openPanel beginSheetModalForWindow:webView.window
        completionHandler:^(NSInteger result) {
            if (result == NSModalResponseOK)
                completionHandler(openPanel.URLs);
            else
                completionHandler(nil);
        }];
}
- (void)webView:(WKWebView *)webView runJavaScriptAlertPanelWithMessage:(NSString *)message
    initiatedByFrame:(WKFrameInfo *)frame completionHandler:(void (^)(void))completionHandler {
    if (webView.window == nil) {
        completionHandler();
        return;
    }
    NSAlert *alert = [[[NSAlert alloc] init] autorelease];
    alert.messageText = message;
    [alert beginSheetModalForWindow:webView.window completionHandler:^(NSModalResponse response) {
        completionHandler();
    }];
}
- (void)webView:(WKWebView *)webView runJavaScriptConfirmPanelWithMessage:(NSString *)message
    initiatedByFrame:(WKFrameInfo *)frame completionHandler:(void (^)(BOOL result))completionHandler {
    if (webView.window == nil) {
        completionHandler(NO);
        return;
    }
    NSAlert *alert = [[[NSAlert alloc] init] autorelease];
    alert.messageText = message;
    [alert addButtonWithTitle:@"OK"];
    [alert addButtonWithTitle:@"Cancel"];
    [alert beginSheetModalForWindow:webView.window completionHandler:^(NSModalResponse response) {
        completionHandler(response == NSAlertFirstButtonReturn);
    }];
}
@end

void* panelSchemeHandlerNew(unsigned int windowId) {
    WailsPanelSchemeHandler *handler = [[WailsPanelSchemeHandler alloc] init];
    handler.windowId = windowId;
    return handler;
}

void* panelDelegateNew(unsigned int windowId, unsigned int panelId) {
    WailsPanelDelegate *delegate = [[WailsPanelDelegate alloc] init];
    delegate.windowId = windowId;
    delegate.panelId = panelId;
    return delegate;
}

void panelObjectRelease(void *object) {
    [(id)object release];
}
