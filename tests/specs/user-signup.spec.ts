import test, { expect, type Page } from '@playwright/test';
import { signupTokens, userGroups, users } from '../data';
import { setAppConfigValues } from '../utils/app-config.util';
import { cleanupBackend } from '../utils/cleanup.util';
import passkeyUtil from '../utils/passkey.util';
import { saveUnsavedChanges } from '../utils/unsaved-changes.util';

async function setSignupMode(
	page: Page,
	mode: 'Disabled' | 'Signup with token' | 'Open Signup',
	signout = true
) {
	await page.goto('/settings/admin/application-configuration');

	await page.getByRole('tab', { name: 'User Creation' }).click();
	const signupMode = page.getByRole('button', { name: 'Enable User Signups' });
	if (!(await signupMode.textContent())?.includes(mode)) {
		await signupMode.click();
		await page.getByRole('option', { name: mode }).click();
		await saveUnsavedChanges(page);
	}

	if (signout) {
		await page.context().clearCookies();
		await page.goto('/login');
	}
}

test.describe('Invite Creation', () => {
	test.beforeEach(async ({ page }) => {
		await cleanupBackend();
		await setSignupMode(page, 'Signup with token', false);
	});

	test('Create a multi-use invite', async ({ page }) => {
		await page.goto('/settings/admin/users');

		await page.getByRole('button', { name: 'Invite', exact: true }).click();

		await page.getByLabel('Who is it for?').fill('Family');
		await page.getByLabel('Several people').check();
		await page.getByLabel('Number of people').fill('8');
		await page.getByLabel('Expires in').click();
		await page.getByRole('option', { name: '30 days' }).click();
		await page.getByLabel('Let them in automatically').check();

		// Groups live behind the collapsed Advanced section
		await page.getByRole('button', { name: 'Advanced' }).click();
		await page.getByLabel('User Groups').click();
		await page.getByRole('option', { name: userGroups.developers.name }).click();
		await page.getByRole('option', { name: userGroups.designers.name }).click();
		await page.getByLabel('User Groups').click();

		await page.getByRole('button', { name: 'Create', exact: true }).click();
		await expect(page.getByTestId('invite-link')).toBeVisible();

		const inviteLink = await page.getByTestId('invite-link').textContent();
		expect(inviteLink).toContain('/st/');

		await page.getByRole('button', { name: 'Close' }).click();
		await page.getByRole('button', { name: 'Create options' }).click();
		await page.getByRole('menuitem', { name: 'Manage invites' }).click();

		const row = page.getByRole('row').last();
		await expect(row.getByRole('cell', { name: 'Family' })).toBeVisible();
		await expect(row.getByRole('cell', { name: '0 / 8' })).toBeVisible();
		await expect(row.getByRole('cell', { name: 'Automatic' })).toBeVisible();
	});

	test('Signup through an automatic invite', async ({ page }) => {
		await page.goto('/settings/admin/users');
		await page.getByRole('button', { name: 'Invite', exact: true }).click();
		await page.getByRole('button', { name: 'Create', exact: true }).click();
		const inviteLink = await page.getByTestId('invite-link').textContent();
		await page.context().clearCookies();

		await page.goto(inviteLink!.trim());
		await expect(page.getByText('You will sign in with your fingerprint')).toBeVisible();
		await page.getByLabel('First name').fill('Auto');
		await page.getByLabel('Last name').fill('Invitee');
		await page.getByLabel('Username').fill('autoinvitee');
		await page.getByLabel('Email').fill('auto.invitee@test.com');
		await page.getByRole('button', { name: 'Sign Up' }).click();

		await page.waitForURL('/signup/add-passkey');
	});

	test('Signup through a manual approval invite lands on the waiting page', async ({ page }) => {
		await page.goto('/settings/admin/users');
		await page.getByRole('button', { name: 'Invite', exact: true }).click();
		await page.getByLabel('I approve each person first').check();
		await page.getByRole('button', { name: 'Create', exact: true }).click();
		const inviteLink = await page.getByTestId('invite-link').textContent();
		await page.context().clearCookies();

		await page.goto(inviteLink!.trim());
		await expect(page.getByText('An admin will review your request')).toBeVisible();
		await page.getByLabel('First name').fill('Manual');
		await page.getByLabel('Last name').fill('Invitee');
		await page.getByLabel('Username').fill('manualinvitee');
		await page.getByLabel('Email').fill('manual.invitee@test.com');
		await page.getByRole('button', { name: 'Sign Up' }).click();

		// After signup the pending user is funneled to the waiting page
		await page.waitForURL('/signup/pending', { timeout: 15000 });
		await expect(page.getByText('You are almost in')).toBeVisible();
	});

	test('Admin approves a pending user who is then let in', async ({ page, browser }) => {
		await page.goto('/settings/admin/users');
		await page.getByRole('button', { name: 'Invite', exact: true }).click();
		await page.getByLabel('I approve each person first').check();
		await page.getByRole('button', { name: 'Create', exact: true }).click();
		const inviteLink = await page.getByTestId('invite-link').textContent();

		// The invitee signs up in a separate context and waits for approval
		const inviteeContext = await browser.newContext();
		const inviteePage = await inviteeContext.newPage();
		await inviteePage.goto(inviteLink!.trim());
		await inviteePage.getByLabel('First name').fill('Wait');
		await inviteePage.getByLabel('Last name').fill('Ing');
		await inviteePage.getByLabel('Username').fill('waitinguser');
		await inviteePage.getByLabel('Email').fill('waiting.user@test.com');
		await inviteePage.getByRole('button', { name: 'Sign Up' }).click();
		await inviteePage.waitForURL('/signup/pending', { timeout: 15000 });

		await expect(page.getByText('Waiting for approval')).toBeVisible();
		await page.getByRole('button', { name: 'Approve' }).first().click();

		// The pending page polls and forwards the invitee once approved
		await inviteePage.waitForURL((url) => url.pathname !== '/signup/pending', {
			timeout: 20000
		});
		await inviteeContext.close();
	});

	test('Invalid invite shows a friendly message', async ({ page }) => {
		await page.context().clearCookies();
		await page.goto('/st/not-a-real-invite');
		await expect(page.getByText('This invite has expired or was already used')).toBeVisible();
		await expect(page.getByLabel('First name')).not.toBeVisible();
	});

	test('Pending user cannot sign in with a passkey', async ({ page }) => {
		await page.goto('/settings/admin/users');
		await page.getByRole('button', { name: 'Invite', exact: true }).click();
		await page.getByLabel('I approve each person first').check();
		await page.getByRole('button', { name: 'Create', exact: true }).click();
		const inviteLink = await page.getByTestId('invite-link').textContent();
		await page.context().clearCookies();

		await page.goto(inviteLink!.trim());
		await page.getByLabel('First name').fill('Blocked');
		await page.getByLabel('Last name').fill('User');
		await page.getByLabel('Username').fill('blockeduser');
		await page.getByLabel('Email').fill('blocked.user@test.com');
		await page.getByRole('button', { name: 'Sign Up' }).click();
		await page.waitForURL('/signup/pending', { timeout: 15000 });

		// Register a passkey, then sign out and try to sign back in
		const util = await passkeyUtil.init(page);
		await util.addPasskey();
		await page.goto('/signup/add-passkey');
		await page.getByRole('button', { name: 'Add Passkey' }).click();
		await page.waitForURL('/signup/pending');

		await page.getByRole('button', { name: 'Sign out' }).click();
		await page.waitForURL('/login');

		await page.getByRole('button', { name: 'Authenticate' }).click();
		await expect(page.getByText('Your account is waiting for approval')).toBeVisible();
	});
});

test.describe('Initial User Signup', () => {
	test.beforeEach(async ({ page }) => {
		await page.context().clearCookies();
	});

	test('Initial Signup - success flow', async ({ page }) => {
		await cleanupBackend({ skipSeed: true });
		await page.goto('/setup');
		await page.getByLabel('First name').fill('Jane');
		await page.getByLabel('Last name').fill('Smith');
		await page.getByLabel('Username').fill('janesmith');
		await page.getByLabel('Email').fill('jane.smith@test.com');
		await page.getByRole('button', { name: 'Sign Up' }).click();
		await page.waitForURL('/signup/add-passkey');
		await expect(page.getByRole('heading', { name: 'Set up your passkey' })).toBeVisible();
	});

	test('Initial Signup - concurrent requests create one administrator', async ({ request }) => {
		await cleanupBackend({ skipSeed: true });

		const requestCount = 20;
		const responses = await Promise.all(
			Array.from({ length: requestCount }, (_, index) =>
				request.post('/api/signup/setup', {
					data: {
						username: `race-admin-${index}`,
						email: `race-admin-${index}@example.invalid`,
						firstName: 'Race',
						lastName: `${index}`
					}
				})
			)
		);

		const successfulResponses = responses.filter((response) => response.status() === 200);
		const rejectedResponses = responses.filter((response) => response.status() === 409);

		expect(successfulResponses).toHaveLength(1);
		expect(rejectedResponses).toHaveLength(requestCount - 1);
		await expect(successfulResponses[0].json()).resolves.toMatchObject({ isAdmin: true });

		const usersResponse = await request.get('/api/users');
		expect(usersResponse.status()).toBe(200);
		const users = await usersResponse.json();
		expect(users.data).toHaveLength(1);
		expect(users.data[0]).toMatchObject({ isAdmin: true });
	});

	test('Initial Signup - setup route unavailable after completion', async ({ page }) => {
		await cleanupBackend();
		await page.goto('/setup');
		await expect(page.getByText('Not found')).toBeVisible();
		await expect(page.getByRole('button', { name: 'Sign Up' })).not.toBeVisible();
	});
});

test.describe('User Signup', () => {
	test.beforeEach(async () => await cleanupBackend());

	test.describe('Signup Flows', () => {
		test('Signup is disabled - shows error message', async ({ page }) => {
			await setSignupMode(page, 'Disabled');

			await page.goto('/signup');

			await expect(page.getByText('User signups are currently disabled')).toBeVisible();
		});

		test('Signup with token - success flow', async ({ page }) => {
			await setSignupMode(page, 'Signup with token');

			await page.goto(`/st/${signupTokens.valid.token}`);

			await page.getByLabel('First name').fill('John');
			await page.getByLabel('Last name').fill('Doe');
			await page.getByLabel('Username').fill('johndoe');
			await page.getByLabel('Email').fill('john.doe@test.com');

			await page.getByRole('button', { name: 'Sign Up' }).click();

			await page.waitForURL('/signup/add-passkey');
			await expect(page.getByRole('heading', { name: 'Set up your passkey' })).toBeVisible();

			const response = await page.request.get('/api/users/me').then((res) => res.json());
			expect(response.userGroups.map((g) => g.id)).toContain(userGroups.developers.id);
		});

		test('Signup with token - invalid token shows error', async ({ page }) => {
			await setSignupMode(page, 'Signup with token');

			await page.goto('/st/invalid-token-123');
			await page.getByLabel('First name').fill('Complete');
			await page.getByLabel('Last name').fill('User');
			await page.getByLabel('Username').fill('completeuser');
			await page.getByLabel('Email').fill('complete.user@test.com');
			await page.getByRole('button', { name: 'Sign Up' }).click();

			await expect(page.getByText('Token is invalid or expired.')).toBeVisible();
		});

		test('Signup with token - no token in URL shows error', async ({ page }) => {
			await setSignupMode(page, 'Signup with token');

			await page.goto('/signup');

			await expect(
				page.getByText('You need a valid invite link to create an account')
			).toBeVisible();
		});

		test('Open signup - success flow', async ({ page }) => {
			await setSignupMode(page, 'Open Signup');

			await page.goto('/signup');

			await expect(page.getByText('Create your account to get started')).toBeVisible();

			await page.getByLabel('First name').fill('Jane');
			await page.getByLabel('Last name').fill('Smith');
			await page.getByLabel('Username').fill('janesmith');
			await page.getByLabel('Email').fill('jane.smith@test.com');

			await page.getByRole('button', { name: 'Sign Up' }).click();

			await page.waitForURL('/signup/add-passkey');
			await expect(page.getByRole('heading', { name: 'Set up your passkey' })).toBeVisible();
		});

		test('Open signup - validation errors', async ({ page }) => {
			await setSignupMode(page, 'Open Signup');

			await page.goto('/signup');

			await page.getByRole('button', { name: 'Sign Up' }).click();

			await expect(page.getByText('Invalid email address').first()).toBeVisible();
		});

		test('Open signup - duplicate email shows error', async ({ page }) => {
			await setSignupMode(page, 'Open Signup');

			await page.goto('/signup');

			await page.getByLabel('First name').fill('Test');
			await page.getByLabel('Last name').fill('User');
			await page.getByLabel('Username').fill('testuser123');
			await page.getByLabel('Email').fill(users.tim.email);

			await page.getByRole('button', { name: 'Sign Up' }).click();

			await expect(page.getByText('Email is already in use.')).toBeVisible();
		});

		test('Open signup - duplicate username shows error', async ({ page }) => {
			await setSignupMode(page, 'Open Signup');

			await page.goto('/signup');

			await page.getByLabel('First name').fill('Test');
			await page.getByLabel('Last name').fill('User');
			await page.getByLabel('Username').fill(users.tim.username);
			await page.getByLabel('Email').fill('newuser@test.com');

			await page.getByRole('button', { name: 'Sign Up' }).click();

			await expect(page.getByText('Username is already in use.')).toBeVisible();
		});

		test('Complete signup flow with passkey creation', async ({ page }) => {
			await setSignupMode(page, 'Open Signup');

			await page.goto('/signup');
			await page.getByLabel('First name').fill('Complete');
			await page.getByLabel('Last name').fill('User');
			await page.getByLabel('Username').fill('completeuser');
			await page.getByLabel('Email').fill('complete.user@test.com');
			await page.getByRole('button', { name: 'Sign Up' }).click();

			await page.waitForURL('/signup/add-passkey');

			await (await passkeyUtil.init(page)).addPasskey('timNew');
			await page.getByRole('button', { name: 'Add Passkey' }).click();

			await page.waitForURL('/settings/account');
			await expect(page.getByText('Single Passkey Configured')).toBeVisible();
		});

		test('Dismiss passkey setup during signup for one day', async ({ page }) => {
			await setSignupMode(page, 'Open Signup');

			await page.goto('/signup');
			await page.getByLabel('First name').fill('Skip');
			await page.getByLabel('Last name').fill('User');
			await page.getByLabel('Username').fill('skipuser');
			await page.getByLabel('Email').fill('skip.user@test.com');
			await page.getByRole('button', { name: 'Sign Up' }).click();

			await page.waitForURL('/signup/add-passkey');
			await expect(page.getByRole('heading', { name: 'Set up your passkey' })).toBeVisible();

			// Cancelling the confirmation must keep passkey setup open
			await page.getByRole('button', { name: 'Skip for now' }).click();
			const dialog = page.getByRole('alertdialog', { name: 'Skip Passkey Setup' });
			await dialog.getByRole('button', { name: 'Cancel' }).click();
			await page.reload();
			await expect(page).toHaveURL('/signup/add-passkey');
			await expect(page.getByRole('heading', { name: 'Set up your passkey' })).toBeVisible();

			// Confirming dismissal allows settings access without creating a passkey
			const dismissedAt = Date.now();
			await page.clock.setFixedTime(dismissedAt);
			await page.getByRole('button', { name: 'Skip for now' }).click();
			await dialog.getByRole('button', { name: 'Skip for now' }).click();

			await page.waitForURL('/settings/account');
			await expect(page.getByText('Passkey missing')).toBeVisible();
			await page.reload();
			await expect(page).toHaveURL('/settings/account');
			await expect(page.getByText('Passkey missing')).toBeVisible();

			// Dismissal persists across settings pages until the one-day grace period expires
			await page.clock.setFixedTime(dismissedAt + 24 * 60 * 60 * 1000 - 1000);
			await page.goto('/settings/apps');
			await expect(page).toHaveURL('/settings/apps');
			await expect(page.getByRole('heading', { name: 'Set up your passkey' })).not.toBeVisible();

			await page.clock.setFixedTime(dismissedAt + 24 * 60 * 60 * 1000);
			await page.reload();
			await expect(page).toHaveURL('/signup/add-passkey');
			await expect(page.getByRole('heading', { name: 'Set up your passkey' })).toBeVisible();
		});

		test('Open signup with required email verification restricts the new account', async ({
			page
		}) => {
			// Required verification needs an SMTP server, but nothing listens on it, so the email sent at signup just fails
			await setAppConfigValues(page.request, {
				emailVerification: 'required',
				smtpHost: 'localhost',
				smtpFrom: 'quad4-id@test.com'
			});
			await setSignupMode(page, 'Open Signup');

			await page.goto('/signup');
			await page.getByLabel('First name').fill('Pending');
			await page.getByLabel('Last name').fill('User');
			await page.getByLabel('Username').fill('pendinguser');
			await page.getByLabel('Email').fill('pending.user@test.com');
			await page.getByRole('button', { name: 'Sign Up' }).click();

			await page.waitForURL('/signup/add-passkey');
			await page.getByRole('button', { name: 'Skip for now' }).click();
			await page
				.getByRole('alertdialog', { name: 'Skip Passkey Setup' })
				.getByRole('button', { name: 'Skip for now' })
				.click();

			await page.waitForURL('/settings/account');
			await expect(page.getByText('Verify your email address to use your account')).toBeVisible();
			await expect(page.getByText('your account will be deleted')).toBeVisible();

			await page.goto('/settings/apps');
			await expect(page).toHaveURL('/settings/account');
		});

		test('Token usage limit is enforced', async ({ page }) => {
			await setSignupMode(page, 'Signup with token');

			await page.goto(`/st/${signupTokens.fullyUsed.token}`);
			await page.getByLabel('First name').fill('Complete');
			await page.getByLabel('Last name').fill('User');
			await page.getByLabel('Username').fill('completeuser');
			await page.getByLabel('Email').fill('complete.user@test.com');
			await page.getByRole('button', { name: 'Sign Up' }).click();

			await expect(page.getByText('Token is invalid or expired.')).toBeVisible();
		});
	});
});
