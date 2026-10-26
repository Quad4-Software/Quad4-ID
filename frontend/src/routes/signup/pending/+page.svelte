<script lang="ts">
	import { goto } from '$app/navigation';
	import SignInWrapper from '#lib/components/login-wrapper.svelte';
	import { Button } from '#lib/components/ui/button/index.ts';
	import { Spinner } from '#lib/components/ui/spinner/index.ts';
	import { m } from '#lib/paraglide/messages.js';
	import UserService from '#lib/services/user-service.ts';
	import WebAuthnService from '#lib/services/webauthn-service.ts';
	import { axiosErrorToast } from '#lib/utils/error-util.ts';
	import { onDestroy, onMount } from 'svelte';
	import LoginLogoErrorSuccessIndicator from '../../login/components/login-logo-error-success-indicator.svelte';

	const userService = new UserService();
	const webauthnService = new WebAuthnService();

	let interval: ReturnType<typeof setInterval>;

	async function checkApproval() {
		try {
			const user = await userService.getCurrent();
			if (!user.approvalPending) {
				await goto('/');
			}
		} catch {
			// Keep polling while the session stays valid
		}
	}

	async function signOut() {
		try {
			await webauthnService.logout();
		} catch (e) {
			axiosErrorToast(e);
		}
		await goto('/login');
	}

	onMount(() => {
		interval = setInterval(checkApproval, 5000);
	});

	onDestroy(() => clearInterval(interval));
</script>

<svelte:head>
	<title>{m.waiting_for_approval()}</title>
</svelte:head>

<SignInWrapper>
	<div class="w-full text-center">
		<div class="flex justify-center">
			<LoginLogoErrorSuccessIndicator error={false} />
		</div>
		<h1 class="font-gloock mt-5 text-3xl font-bold sm:text-4xl">
			{m.you_are_almost_in()}
		</h1>
		<p class="text-muted-foreground mt-2">
			{m.waiting_for_approval_description()}
		</p>
		<div class="mt-8 flex justify-center">
			<Spinner class="size-8" />
		</div>
		<div class="mt-10 flex w-full justify-center">
			<Button variant="secondary" onclick={signOut}>{m.sign_out()}</Button>
		</div>
	</div>
</SignInWrapper>
