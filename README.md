## Project Overview

**Contrast Checker** is a web application that calculates the contrast ratio between foreground and background color combinations, determining their compliance.
Users can search and filter color combinations, and download the results as a CSV file.

## Features

- **Real-Time Contrast Calculation**: Instantly calculates and displays the contrast ratio between selected foreground and background colors.
- **Multi-Language Support**: Default language is English, with the option to switch to Japanese.
- **Dark Mode**: Toggle between light and dark themes to suit user preferences.
- **Search and Filter**: Search by color name and filter results by WCAG compliance level.
- **CSV Download**: Download the contrast results as a CSV file for further analysis.
- **Responsive Design**: Optimized for various devices, including desktops and mobile devices.
- **Accessibility Focused**: Enhanced keyboard navigation and screen reader support to ensure accessibility for all users.
- **Toast Notifications**: Provides instant feedback for user actions like theme and language changes.

## Installation

### Prerequisites

- [Go](https://golang.org/dl/) (version 1.16 or higher)

### Steps

1. **Clone the Repository**

   ```bash
   cd contrast-checker
   ```

2. **Install Dependencies**

   No additional dependencies are required. However, you can initialize Go modules if necessary.

   ```bash
   go mod tidy
   ```

3. **Verify `colors.json`**

   Ensure that the `colors.json` file exists in the project directory. You can customize the color data as needed.

## Usage

1. **Start the Server**

   Navigate to the project directory and run:

   ```bash
   go run main.go
   ```

   You should see the following message:

   ```
   Server running at http://localhost:8080/
   ```

2. **Access the Application**

   Open your web browser and go to `http://localhost:8080/`. The application should open automatically, but if it doesn't, you can manually navigate to the URL.

3. **Utilize Features**

   - **Color Selection and Contrast Calculation**: Choose foreground and background colors to see the contrast ratio in real-time.
   - **Search**: Use the search bar to find color combinations by name.
   - **Filter**: Filter results based on WCAG compliance levels (AAA, AA, Fail).
   - **Language Toggle**: Switch between English and Japanese using the "EN / JP" button.
   - **Dark Mode**: Toggle between light and dark themes using the theme button.
   - **CSV Download**: Click "Download Results as CSV" to export the contrast data.
   - **Modal Window**: View fixable color combinations in a modal for easier management.

## Testing and Verification

- **Language Toggle**: Ensure that switching between English and Japanese updates all relevant text on the page.
- **Dark Mode**: Verify that the theme toggles correctly and that the preference is saved across sessions.
- **Contrast Calculations**: Select various color combinations to confirm accurate contrast ratio calculations and proper WCAG categorization.
- **Responsive Design**: Resize the browser window or access the application on different devices to ensure the layout adjusts appropriately.
- **Accessibility**: Navigate the application using only the keyboard and test with screen readers to confirm accessibility features.
- **CSV Download**: Download the CSV file and verify that all data is correctly formatted and complete.
- **Toast Notifications**: Perform actions like theme and language changes to see if toast notifications appear and disappear as expected.
