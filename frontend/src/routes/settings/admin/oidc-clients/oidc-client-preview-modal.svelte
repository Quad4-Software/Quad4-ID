<script lang="ts">
	import CopyToClipboard from '#lib/components/copy-to-clipboard.svelte';
	import TokenPreviewInputs from '#lib/components/token-preview-inputs.svelte';
	import { Badge } from '#lib/components/ui/badge/index.ts';
	import { Button } from '#lib/components/ui/button/index.ts';
	import * as Dialog from '#lib/components/ui/dialog/index.ts';

	import { Spinner } from '#lib/components/ui/spinner/index.ts';
	import * as Tabs from '#lib/components/ui/tabs/index.ts';
	import { m } from '#lib/paraglide/messages.js';
	import OidcService from '#lib/services/oidc-service.ts';
	import { getAxiosErrorMessage } from '#lib/utils/error-util.ts';
	import { cn } from '#lib/utils/style.ts';
	import { LucideBraces, LucideCopy, LucideList } from '@lucide/svelte';

	type Claims = Record<string, unknown>;

	let {
		open = $bindable(),
		clientId
	}: {
		open: boolean;
		clientId: string;
	} = $props();

	const oidcService = new OidcService();

	// Claims holding a Unix timestamp, which are shown with a readable date next to them
	const TIMESTAMP_CLAIMS = new Set(['exp', 'iat', 'nbf', 'auth_time', 'updated_at']);

	// The ID token and userinfo are null when openid isn't requested, because a real authorization wouldn't issue them
	let previewData = $state<{
		idToken: Claims | null;
		accessToken: Claims;
		userInfo: Claims | null;
	} | null>(null);
	let isLoading = $state(false);
	let userId = $state('');
	let scopes: string[] = $state(['openid', 'email', 'profile']);
	let errorMessage: string | null = $state(null);
	let view = $state<'claims' | 'json'>('claims');
	let activeTab = $state<'idToken' | 'accessToken' | 'userInfo'>('idToken');

	// Each request gets a number so that responses arriving out of order don't overwrite newer ones
	let latestRequest = 0;

	const activeData = $derived(previewData?.[activeTab]);

	async function loadPreviewData(userId: string, scope: string) {
		const request = ++latestRequest;
		isLoading = true;
		errorMessage = null;

		try {
			const data = await oidcService.getClientPreview(clientId, userId, scope);
			if (request === latestRequest) previewData = data;
		} catch (e) {
			if (request !== latestRequest) return;
			errorMessage = getAxiosErrorMessage(e);
			previewData = null;
		} finally {
			if (request === latestRequest) isLoading = false;
		}
	}

	// Timestamps arrive either as Unix seconds or as ISO strings depending on the claim, so both are accepted
	function formatTimestamp(value: unknown) {
		const date =
			typeof value === 'number'
				? new Date(value * 1000)
				: typeof value === 'string'
					? new Date(value)
					: null;
		return date && !isNaN(date.getTime()) ? date.toLocaleString() : null;
	}

	// Reload the preview whenever the selected user or the scopes change
	$effect(() => {
		if (open && userId) {
			loadPreviewData(userId, scopes.join(' '));
		}
	});
</script>

{#snippet claimValue(key: string, value: unknown)}
	{#if Array.isArray(value) && value.every((item) => typeof item !== 'object')}
		<div class="flex flex-wrap gap-1.5">
			{#each value as item, i (i)}
				<Badge variant="secondary" class="h-auto max-w-full font-mono break-all whitespace-normal"
					>{String(item)}</Badge
				>
			{:else}
				<span class="text-muted-foreground font-mono text-xs">[]</span>
			{/each}
		</div>
	{:else if value !== null && typeof value === 'object'}
		<pre class="font-mono text-xs break-all whitespace-pre-wrap">{JSON.stringify(
				value,
				null,
				2
			)}</pre>
	{:else if value === '' || value === null}
		<span class="text-muted-foreground font-mono text-xs">{value === null ? 'null' : '""'}</span>
	{:else}
		{@const formattedDate = TIMESTAMP_CLAIMS.has(key) ? formatTimestamp(value) : null}
		<CopyToClipboard value={String(value)}>
			<span class="font-mono text-xs break-all">{String(value)}</span>
		</CopyToClipboard>
		{#if formattedDate}
			<span class="text-muted-foreground ml-2 text-xs">{formattedDate}</span>
		{/if}
	{/if}
{/snippet}

<Dialog.Root bind:open>
	<Dialog.Content class="flex h-[min(90vh,46rem)] flex-col sm:max-w-4xl">
		<Dialog.Header>
			<Dialog.Title>{m.oidc_data_preview()}</Dialog.Title>
			<Dialog.Description>
				{m.preview_the_oidc_data_that_would_be_sent_for_different_users()}
			</Dialog.Description>
		</Dialog.Header>

		<TokenPreviewInputs bind:userId bind:scopes />

		<div class="flex min-h-0 flex-1 flex-col gap-3">
			<div class="flex items-center justify-between gap-3 border-b">
				<!-- The token tabs scroll on narrow screens so the view toggle and copy button stay inside the dialog -->
				<Tabs.Root bind:value={activeTab} class="min-w-0 [scrollbar-width:none] overflow-x-auto">
					<Tabs.List variant="line">
						<Tabs.Trigger value="idToken">{m.id_token()}</Tabs.Trigger>
						<Tabs.Trigger value="accessToken">{m.access_token()}</Tabs.Trigger>
						<Tabs.Trigger value="userInfo">{m.userinfo()}</Tabs.Trigger>
					</Tabs.List>
				</Tabs.Root>
				<div class="flex shrink-0 items-center gap-1 pb-1">
					<Tabs.Root bind:value={view}>
						<Tabs.List class="h-8">
							<Tabs.Trigger value="claims" class="px-2" aria-label={m.claims()}>
								<LucideList class="size-3.5" />
							</Tabs.Trigger>
							<Tabs.Trigger value="json" class="px-2" aria-label="JSON">
								<LucideBraces class="size-3.5" />
							</Tabs.Trigger>
						</Tabs.List>
					</Tabs.Root>
					<CopyToClipboard value={JSON.stringify(activeData, null, 2)}>
						<Button
							size="icon-sm"
							variant="ghost"
							aria-label={m.copy_all()}
							disabled={!previewData}
						>
							<LucideCopy class="size-3.5" />
						</Button>
					</CopyToClipboard>
				</div>
			</div>

			<!-- The frame has a fixed size so the dialog doesn't jump in height between tabs, views and reloads -->
			<div
				class={cn(
					'min-h-0 flex-1 overflow-y-auto rounded-2xl border transition-opacity',
					isLoading && previewData && 'opacity-50'
				)}
			>
				{#if errorMessage && !isLoading}
					<div class="flex h-full flex-col items-center justify-center p-6 text-center">
						<p class="font-medium">{m.error()}</p>
						<p class="text-muted-foreground max-w-sm">{errorMessage}</p>
					</div>
				{:else if !previewData}
					<div class="flex h-full items-center justify-center">
						<Spinner class="size-6" />
					</div>
				{:else if !activeData}
					<p class="text-muted-foreground px-6 py-10 text-center">
						{m.only_issued_with_openid_scope()}
					</p>
				{:else if view === 'json'}
					<pre class="p-4 font-mono text-xs break-all whitespace-pre-wrap">{JSON.stringify(
							activeData,
							null,
							2
						)}</pre>
				{:else}
					<dl class="divide-y" data-testid="preview-claims">
						{#each Object.entries(activeData) as [key, value] (key)}
							<div class="grid gap-1 px-4 py-2.5 sm:grid-cols-[11rem_minmax(0,1fr)] sm:gap-4">
								<dt class="text-muted-foreground font-mono text-xs leading-5">{key}</dt>
								<dd class="min-w-0 leading-5">{@render claimValue(key, value)}</dd>
							</div>
						{/each}
					</dl>
				{/if}
			</div>
		</div>
	</Dialog.Content>
</Dialog.Root>
