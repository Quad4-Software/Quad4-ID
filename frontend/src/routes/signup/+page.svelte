<script lang="ts">
	import SignInWrapper from '#lib/components/login-wrapper.svelte';
	import SignupForm from '#lib/components/signup/signup-form.svelte';
	import { Button } from '#lib/components/ui/button/index.ts';
	import { m } from '#lib/paraglide/messages.js';
	import UserService from '#lib/services/user-service.ts';
	import appConfigStore from '#lib/stores/application-configuration-store.ts';
	import userStore from '#lib/stores/user-store.ts';
	import type { InviteStatus } from '#lib/types/signup-token.type.ts';
	import type { UserSignUp } from '#lib/types/user.type.ts';
	import { getAxiosErrorMessage } from '#lib/utils/error-util.ts';
	import { tryCatch } from '#lib/utils/try-catch-util.ts';
	import { goto } from '$app/navigation';
	import { LucideChevronLeft } from '@lucide/svelte';
	import { onMount } from 'svelte';
	import { fade } from 'svelte/transition';
	import LoginLogoErrorSuccessIndicator from '../login/components/login-logo-error-success-indicator.svelte';

	let { data } = $props();
	const userService = new UserService();

	let error: string | undefined = $state();
	let invite: InviteStatus | null = $state(null);
	let inviteChecked = $state(false);

	async function handleSignup(userData: UserSignUp) {
		const result = await tryCatch(userService.signup({ ...userData, token: data.token }));

		if (result.error) {
			error = getAxiosErrorMessage(result.error);
			return false;
		}

		await userStore.setUser(result.data);

		// A user waiting for approval goes straight to the waiting page instead of the home redirect
		goto(result.data.approvalPending ? '/signup/pending' : '/');
		return true;
	}

	onMount(async () => {
		if (!$appConfigStore.allowUserSignups || $appConfigStore.allowUserSignups === 'disabled') {
			error = m.user_signups_are_disabled();
			return;
		}

		// For token-based signups, check if we have a valid token
		if ($appConfigStore.allowUserSignups === 'withToken' && !data.token) {
			error = m.signup_requires_valid_token();
			return;
		}

		if (data.token) {
			const result = await tryCatch(userService.getInviteStatus(data.token));
			if (result.error || !result.data.valid) {
				error = m.invite_expired_or_used();
			} else {
				invite = result.data;
			}
		}
		inviteChecked = true;
	});
</script>

<svelte:head>
	<title>{m.signup()}</title>
</svelte:head>

<SignInWrapper>
	<div class="flex justify-center">
		<LoginLogoErrorSuccessIndicator error={!!error} />
	</div>

	{#if data.token && invite}
		<h1 class="font-gloock mt-5 text-3xl font-bold sm:text-4xl">
			{m.you_are_invited_to_appname({ appName: $appConfigStore.appName })}
		</h1>
	{:else}
		<h1 class="font-gloock mt-5 text-3xl font-bold sm:text-4xl">
			{m.signup_to_appname({ appName: $appConfigStore.appName })}
		</h1>
	{/if}

	{#if error}
		<p class="text-muted-foreground mt-2" in:fade>
			{error}
		</p>
		<Button class="mt-10" href="/login">{m.go_to_login()}</Button>
	{:else if !inviteChecked}
		<p class="text-muted-foreground mt-2" in:fade>
			{m.checking_your_invite()}
		</p>
	{:else}
		<p class="text-muted-foreground mt-2" in:fade>
			{#if invite}
				{m.you_sign_in_with_passkey()}
				{#if invite.requireApproval}
					{m.admin_reviews_before_signin()}
				{/if}
			{:else}
				{m.create_your_account_to_get_started()}
			{/if}
		</p>
		<SignupForm callback={handleSignup} />
		<div class="mt-10 flex w-full items-center justify-between gap-2">
			<a class="text-muted-foreground mt-5 flex text-sm" href="/login"
				><LucideChevronLeft class="size-5" /> {m.back()}</a
			>
			<Button type="submit" form="sign-up-form" onclick={() => (error = undefined)}
				>{m.signup()}</Button
			>
		</div>
	{/if}
</SignInWrapper>
