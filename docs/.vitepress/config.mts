import { defineConfig } from 'vitepress'

// VitePress configuration for the Northwatch documentation site.
//
// The site is organized along the Diátaxis framework
// (https://diataxis.fr/): Tutorials (learning), How-to guides (tasks),
// Reference (lookup), and Explanation (understanding). The `base` must match the
// GitHub Pages project path (https://b42labs.github.io/northwatch/), which is
// derived from the repository name; without a matching base the asset and link
// URLs 404 on the published site.
export default defineConfig({
  title: 'Northwatch',
  description:
    'Real-time analyzer, visualizer, debugger and monitor for OVN — browse the Northbound and Southbound databases, correlate across them, and search everything from one service.',
  base: '/northwatch/',
  cleanUrls: true,
  lastUpdated: true,

  // Internal links are still verified (a broken one fails the build); only the
  // `http://localhost:8080` example URLs in the guides are exempt.
  ignoreDeadLinks: 'localhostLinks',

  themeConfig: {
    nav: [
      { text: 'Tutorials', link: '/tutorials/' },
      { text: 'How-to', link: '/how-to/' },
      { text: 'Reference', link: '/reference/' },
      { text: 'Explanation', link: '/explanation/' },
    ],

    sidebar: {
      '/tutorials/': [
        {
          text: 'Tutorials',
          items: [
            { text: 'Overview', link: '/tutorials/' },
            { text: 'Getting started', link: '/tutorials/getting-started' },
            {
              text: 'Investigate with Omnisearch',
              link: '/tutorials/investigate-with-omnisearch',
            },
            {
              text: 'Explore a deployment offline',
              link: '/tutorials/explore-a-deployment-offline',
            },
          ],
        },
      ],
      '/how-to/': [
        {
          text: 'How-to guides',
          items: [
            { text: 'Overview', link: '/how-to/' },
            {
              text: 'Install on Debian/Ubuntu',
              link: '/how-to/install-debian',
            },
            { text: 'Run the local lab', link: '/how-to/run-the-local-lab' },
            {
              text: 'Connect to a Raft cluster',
              link: '/how-to/connect-to-a-raft-cluster',
            },
            {
              text: 'Monitor multiple clusters',
              link: '/how-to/monitor-multiple-clusters',
            },
            {
              text: 'Enrich with OpenStack',
              link: '/how-to/enrich-with-openstack',
            },
            {
              text: 'Enrich with Kubernetes',
              link: '/how-to/enrich-with-kubernetes',
            },
            {
              text: 'Search with Omnisearch',
              link: '/how-to/search-with-omnisearch',
            },
            {
              text: 'Trace a packet path',
              link: '/how-to/trace-a-packet-path',
            },
            {
              text: 'Diagnose port bindings',
              link: '/how-to/diagnose-port-bindings',
            },
            { text: 'Audit ACLs', link: '/how-to/audit-acls' },
            {
              text: 'Capture & serve a snapshot',
              link: '/how-to/capture-and-serve-a-snapshot',
            },
            {
              text: 'Tune the initial load',
              link: '/how-to/tune-the-initial-load',
            },
            {
              text: 'Enable write operations',
              link: '/how-to/enable-write-operations',
            },
            { text: 'Configure alerts', link: '/how-to/configure-alerts' },
            {
              text: 'Scrape Prometheus metrics',
              link: '/how-to/scrape-prometheus-metrics',
            },
            { text: 'Explore the API', link: '/how-to/explore-the-api' },
          ],
        },
      ],
      '/reference/': [
        {
          text: 'Reference',
          items: [
            { text: 'Overview', link: '/reference/' },
            { text: 'CLI flags & env vars', link: '/reference/cli' },
            { text: 'Configuration file', link: '/reference/configuration' },
            { text: 'HTTP API', link: '/reference/api' },
            { text: 'Capabilities', link: '/reference/capabilities' },
            { text: 'Prometheus metrics', link: '/reference/metrics' },
            { text: 'Make targets', link: '/reference/make-targets' },
            {
              text: 'Project structure',
              link: '/reference/project-structure',
            },
          ],
        },
      ],
      '/explanation/': [
        {
          text: 'Explanation',
          items: [
            { text: 'Overview', link: '/explanation/' },
            { text: 'Architecture', link: '/explanation/architecture' },
            {
              text: 'Correlation & search',
              link: '/explanation/correlation-and-search',
            },
            {
              text: 'The capability model',
              link: '/explanation/capability-model',
            },
            { text: 'Enrichment', link: '/explanation/enrichment' },
            {
              text: 'History & snapshots',
              link: '/explanation/history-and-snapshots',
            },
            { text: 'Write safety', link: '/explanation/write-safety' },
            {
              text: 'Large deployments',
              link: '/explanation/large-deployments',
            },
            { text: 'Roadmap', link: '/explanation/roadmap' },
          ],
        },
      ],
    },

    search: {
      provider: 'local',
    },

    socialLinks: [
      { icon: 'github', link: 'https://github.com/B42Labs/northwatch' },
    ],

    editLink: {
      pattern: 'https://github.com/B42Labs/northwatch/edit/main/docs/:path',
      text: 'Edit this page on GitHub',
    },
  },
})
