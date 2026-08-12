// Compiled only on macOS. cgo hands every .m file in the package to the C
// compiler regardless of Go build tags, so guard the whole unit on __APPLE__
// to keep Linux/Windows builds (which also build package webview) clean.
#ifdef __APPLE__

#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>
#import <objc/runtime.h>

#include "webview_nav_darwin.h"

// MassNavDelegate keeps the window on the app's own pages: the page served
// from the app's local origin IS the UI, so following an external link
// in-window would replace the app with a remote site and strand the user
// there (a bare webview has no browser chrome to come back with). Web
// navigations that leave the app's origin go to the OS browser instead,
// covering both in-place navigations (target=_self) and new-window requests
// (target=_blank, which a bare webview would otherwise silently drop).
//
// The vendored webview layer installs its own WKUIDelegate (the file-input
// open panel) and WKWebView holds delegates weakly, so this object retains
// the delegates it displaces and forwards every selector it does not
// implement itself to them.
@interface MassNavDelegate : NSObject <WKNavigationDelegate, WKUIDelegate>
@property (nonatomic, copy) NSString *origin;
@property (nonatomic, retain) id navInner;
@property (nonatomic, retain) id uiInner;
@end

@implementation MassNavDelegate

// isExternal defers to the shared Go rule so all three platforms agree.
- (BOOL)isExternal:(NSString *)uri {
	if (uri == nil || self.origin == nil) {
		return NO;
	}
	return goShouldOpenExternal((char *)[uri UTF8String],
	                            (char *)[self.origin UTF8String]) != 0;
}

- (void)openExternal:(NSString *)uri {
	NSURL *url = [NSURL URLWithString:uri];
	if (url == nil || ![[NSWorkspace sharedWorkspace] openURL:url]) {
		NSLog(@"webview: opening %@ externally failed", uri);
	}
}

- (void)webView:(WKWebView *)webView
    decidePolicyForNavigationAction:(WKNavigationAction *)navigationAction
                    decisionHandler:(void (^)(WKNavigationActionPolicy))decisionHandler {
	// A nil targetFrame means the navigation wants a new window (target=_blank
	// or window.open) — the counterpart of WebKitGTK's NEW_WINDOW_ACTION.
	// Sub-frame navigations are left alone: they cannot strand the user.
	WKFrameInfo *target = [navigationAction targetFrame];
	BOOL topLevel = target == nil || [target isMainFrame];
	NSString *uri = [[[navigationAction request] URL] absoluteString];
	if (topLevel && [self isExternal:uri]) {
		[self openExternal:uri];
		decisionHandler(WKNavigationActionPolicyCancel);
		return;
	}
	id<WKNavigationDelegate> inner = self.navInner;
	if ([inner respondsToSelector:_cmd]) {
		[inner webView:webView
		    decidePolicyForNavigationAction:navigationAction
		                    decisionHandler:decisionHandler];
		return;
	}
	decisionHandler(WKNavigationActionPolicyAllow);
}

// createWebViewWithConfiguration catches new-window requests that never reach
// the policy handler. Returning nil declines the popup, which is what a
// single-window app wants for anything that stayed on its own origin.
- (WKWebView *)webView:(WKWebView *)webView
    createWebViewWithConfiguration:(WKWebViewConfiguration *)configuration
               forNavigationAction:(WKNavigationAction *)navigationAction
                    windowFeatures:(WKWindowFeatures *)windowFeatures {
	NSString *uri = [[[navigationAction request] URL] absoluteString];
	if ([self isExternal:uri]) {
		[self openExternal:uri];
		return nil;
	}
	id<WKUIDelegate> inner = self.uiInner;
	if ([inner respondsToSelector:_cmd]) {
		return [inner webView:webView
		    createWebViewWithConfiguration:configuration
		               forNavigationAction:navigationAction
		                    windowFeatures:windowFeatures];
	}
	return nil;
}

- (BOOL)respondsToSelector:(SEL)selector {
	return [super respondsToSelector:selector] ||
	       [self.navInner respondsToSelector:selector] ||
	       [self.uiInner respondsToSelector:selector];
}

- (id)forwardingTargetForSelector:(SEL)selector {
	if ([self.navInner respondsToSelector:selector]) {
		return self.navInner;
	}
	if ([self.uiInner respondsToSelector:selector]) {
		return self.uiInner;
	}
	return [super forwardingTargetForSelector:selector];
}

@end

static const char *kMassNavDelegateKey = "mass_nav_delegate";

// findWebView returns the first WKWebView in the view tree. The vendored
// engine makes the WKWebView the window's contentView directly; the walk is
// there so a future wrapper view doesn't silently disable the hook.
static WKWebView *findWebView(NSView *view) {
	if (view == nil) {
		return nil;
	}
	if ([view isKindOfClass:[WKWebView class]]) {
		return (WKWebView *)view;
	}
	for (NSView *sub in [view subviews]) {
		WKWebView *found = findWebView(sub);
		if (found != nil) {
			return found;
		}
	}
	return nil;
}

void installExternalNav(void *win, const char *origin) {
	NSWindow *window = (NSWindow *)win;
	if (window == nil || origin == NULL) {
		return;
	}
	WKWebView *webView = findWebView([window contentView]);
	if (webView == nil) {
		return;
	}

	MassNavDelegate *delegate = [[MassNavDelegate alloc] init];
	delegate.origin = [NSString stringWithUTF8String:origin];
	delegate.navInner = [webView navigationDelegate];
	delegate.uiInner = [webView UIDelegate];
	// WKWebView refers to its delegates weakly; retain ours for the webview's
	// lifetime.
	objc_setAssociatedObject(webView, kMassNavDelegateKey, delegate,
	                         OBJC_ASSOCIATION_RETAIN_NONATOMIC);
	[webView setNavigationDelegate:delegate];
	[webView setUIDelegate:delegate];
}

#endif // __APPLE__
