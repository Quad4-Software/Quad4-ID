import OidcClaimMappingPolicyService from '#lib/services/oidc-claim-mapping-policy-service.ts';
import OidcService from '#lib/services/oidc-service.ts';
import type { OidcDiscoveryConfiguration } from '#lib/types/oidc.type.ts';
import { decodeClientIdParam } from '#lib/utils/client-id-util.ts';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, params }) => {
	const oidcService = new OidcService();
	const policyService = new OidcClaimMappingPolicyService();
	const id = decodeClientIdParam(params.id);

	// Admins without access to claim mapping policies still get the page, just without the policy select
	const [client, scimServiceProvider, oidcConfiguration, claimMappingPolicies] = await Promise.all([
		oidcService.getClient(id),
		oidcService.getScimResourceProvider(id).catch(() => undefined),
		fetch('/.well-known/openid-configuration').then(
			(response) => response.json() as Promise<OidcDiscoveryConfiguration>
		),
		policyService
			.listClaimMappingPolicies({
				pagination: { page: 1, limit: 20 },
				sort: { column: 'isDefault', direction: 'desc' }
			})
			.then((policies) => policies.data)
			.catch(() => undefined)
	]);

	// The select labels the selected policy from its items, so a policy outside the first page is loaded on its own
	if (
		claimMappingPolicies &&
		client.claimMappingPolicyId &&
		!claimMappingPolicies.some((policy) => policy.id === client.claimMappingPolicyId)
	) {
		claimMappingPolicies.push(
			await policyService.getClaimMappingPolicy(client.claimMappingPolicyId)
		);
	}

	return {
		client,
		scimServiceProvider,
		oidcConfiguration,
		claimMappingPolicies
	};
};
