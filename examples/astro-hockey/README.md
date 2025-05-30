
# Astro example application for Taronja Gateway

An example application built with Astro that demonstrates how to use Taronja Gateway for authentication and authorization.

This is the page of a Hockey team. Home and Schedule pages are public, while the Roster page is protected and requires authentication.

# Setup

Create a `.env` file in the root of the project with the following content. Only populate the providers that you want to use.

```env
GOOGLE_CLIENT_ID=AAAA
GOOGLE_CLIENT_SECRET=BBBB

GITHUB_CLIENT_ID=CCCC
GITHUB_CLIENT_SECRET=DDDD
```

# Run the example application

```bash
# Install dependencies
npm install
# Start the application
npm run gateway
```

# Access the application

Open your browser and navigate to `http://localhost:8080` to access the Astro example application.
